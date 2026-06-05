import ctypes
import sys
import os
from pathlib import Path
from sign_dll import sign_dll


def _load_lib() -> ctypes.CDLL:
    base_path = f"{Path(__file__).parent.parent}{os.sep}build{os.sep}blobber."
    if sys.platform == "win32":
        base_path += "dll"
        sign_dll(base_path)
    elif sys.platform == "darwin":
        base_path += "dylib"
    else:
        base_path += "so"

    lib = ctypes.CDLL(base_path)

    # int64_t BlobOpen(char* readFile, char* writeFile, int64_t* compression);
    lib.BlobOpen.argtypes = [ctypes.c_char_p,
                             ctypes.c_char_p,
                             ctypes.POINTER(ctypes.c_int64)]
    lib.BlobOpen.restype = ctypes.c_int64

    # int64_t BlobCompress(char* filePath, uint64_t* fileLength, uint64_t* filePosition, uint64_t* fileLastModifiedNs, char** fileHash, int64_t* fileChanged);
    lib.BlobCompress.argtypes = [ctypes.c_char_p,
                                 ctypes.POINTER(ctypes.c_uint64),
                                 ctypes.POINTER(ctypes.c_uint64),
                                 ctypes.POINTER(ctypes.c_uint64),
                                 ctypes.POINTER(ctypes.c_char_p),
                                 ctypes.POINTER(ctypes.c_int64)]
    lib.BlobCompress.restype = ctypes.c_int64

    # int64_t BlobDecompress(char* targetPath, uint64_t* position, uint64_t fileLength);
    lib.BlobDecompress.argtypes = [ctypes.c_char_p,
                                   ctypes.POINTER(ctypes.c_uint64),
                                   ctypes.c_uint64]
    lib.BlobDecompress.restype = ctypes.c_int64

    # int64_t BlobClose(void);
    lib.BlobClose.argtypes = []
    lib.BlobClose.restype = ctypes.c_int64

    # int64_t BlobCloseWithStatistics(char** compressionRate);
    lib.BlobCloseWithStatistics.argtypes = [ctypes.POINTER(ctypes.c_char_p)]
    lib.BlobCloseWithStatistics.restype = ctypes.c_int64

    # char* GetError(int64_t* hasNext);
    lib.GetError.argtypes = [ctypes.POINTER(ctypes.c_int64)]
    lib.GetError.restype = ctypes.c_void_p

    return lib


_LIB: ctypes.CDLL | None = None

def _get_lib() -> ctypes.CDLL:
    global _LIB
    if _LIB is None:
        _LIB = _load_lib()
    return _LIB

class BlobSession:
    def __init__(self):
        self._lib  = _get_lib()

    def open_for_writing(self, path: str, compression_level: int = 7):
        """
        Opens the blobber.dll for writing to a blob file.

        :param path: the path of the blob file
        :param compression_level: the zstd compression level that should be used
        """
        if compression_level is not None:
            compression_level = ctypes.byref(ctypes.c_int64(compression_level))
        success = self._lib.BlobOpen(None, path.encode("UTF-8"), compression_level)
        if not success:
            raise RuntimeError(self.__read_error())

    def open_for_reading(self, path: str):
        """
        Opens the blobber.dll for reading from a blob file.

        :param path: the path of the blob file
        """
        success = self._lib.BlobOpen(path.encode("UTF-8"), None, None)
        if not success:
            raise RuntimeError(self.__read_error())

    def read_file_to_blob(self, path: str,
                          file_length: int = 0,
                          file_position: int = 0,
                          file_ts: int = 0,
                          hash_string: str = None
                          ) -> tuple[bool | None, int, int, int, str]:
        """
        Reads in the file at the given path and appends it to the opened blob file. This function will raise an
        exception if something went wrong during the write or if a write file hasn't been opened yet.

        :param path: the path of the file to read
        :param file_length: the length of the file in bytes
        :param file_position: the position of the file in the blob
        :param file_ts: the last-edited date of the file in UNIX-TS nanoseconds
        :param hash_string: the hash string to compare to, set to None if no version to compare to
        :return: fileChanged (or None if file was ignored), fileLength, filePosition, fileEditedTS, fileHash
        """
        file_length_c = ctypes.c_uint64(file_length)
        file_position_c = ctypes.c_uint64(file_position)
        file_ts_c = ctypes.c_uint64(file_ts)

        hash_ptr = ctypes.c_char_p(None if hash_string is None else hash_string.encode("UTF-8"))
        file_changed = ctypes.c_int64(0)

        success = self._lib.BlobCompress(path.encode("UTF-8"),
                                         ctypes.byref(file_length_c),
                                         ctypes.byref(file_position_c),
                                         ctypes.byref(file_ts_c),
                                         ctypes.byref(hash_ptr),
                                         ctypes.byref(file_changed))
        if not success:
            raise RuntimeError(self.__read_error())

        return (None if file_changed.value == -1 else bool(file_changed.value),
                file_length_c.value, file_position_c.value, file_ts_c.value,
                hash_ptr.value.decode("UTF-8"))

    def read_file_from_blob(self, path: str,
                            file_length: int = 0,
                            file_position: int = 0,
                            ) -> int:
        """
        Reads the specified bytes into the given file path.

        :param path: target file to write the bytes to
        :param file_length: the length of the new file in bytes
        :param file_position: the position in the blob where the file resides
        :return: the new position where the blob pointer is
        """
        file_position_c = ctypes.c_uint64(file_position)

        success = self._lib.BlobDecompress(path.encode("UTF-8"),
                                           ctypes.byref(file_position_c),
                                           ctypes.c_uint64(file_length))
        if not success:
            raise RuntimeError(self.__read_error())

        return file_position_c.value


    def close_blob_file(self):
        """
        Closes the opened blob file, if there is an error the function throws it.
        """
        success = self._lib.BlobClose()
        if not success:
            raise RuntimeError(self.__read_error())

    def close_blob_file_with_stats(self) -> str:
        """
        Closes the opened blob file, if there is an error the function throws it.
        """
        hash_ptr = ctypes.c_char_p(None)

        success = self._lib.BlobCloseWithStatistics(ctypes.byref(hash_ptr))
        if not success:
            raise RuntimeError(self.__read_error())

        return hash_ptr.value.decode("UTF-8")

    def __read_error(self) -> str:
        has_next = ctypes.c_int64(1)
        result = ""

        while has_next.value:
            txt_ptr = self._lib.GetError(ctypes.byref(has_next))
            result += ctypes.string_at(txt_ptr).decode("utf-8")

        return result