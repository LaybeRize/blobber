from __future__ import annotations

import ctypes
import glob
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

    # --------------- Direct Access Functions ---------------

    # int64_t BlobOpen(char* readFile, char* writeFile, int64_t* compression);
    lib.BlobOpen.argtypes = [ctypes.c_char_p,
                             ctypes.c_char_p,
                             ctypes.POINTER(ctypes.c_int64)]
    lib.BlobOpen.restype = ctypes.c_int64

    # int64_t BlobCompress(char* filePath, uint64_t* fileLength, uint64_t* filePosition, int64_t* fileLastModifiedNs, char** fileHash, int64_t* fileChanged);
    lib.BlobCompress.argtypes = [ctypes.c_char_p,
                                 ctypes.POINTER(ctypes.c_uint64),
                                 ctypes.POINTER(ctypes.c_uint64),
                                 ctypes.POINTER(ctypes.c_int64),
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

    # --------------- Version Management Functions ---------------

    # int64_t LoadOverview(char* path);
    lib.LoadOverview.argtypes = [ctypes.c_char_p]
    lib.LoadOverview.restype = ctypes.c_int64

    # int64_t CloseOverview();
    lib.CloseOverview.argtypes = []
    lib.CloseOverview.restype = ctypes.c_int64

    # int64_t RegisterNewRepository(char* repositoryName);
    lib.RegisterNewRepository.argtypes = [ctypes.c_char_p]
    lib.RegisterNewRepository.restype = ctypes.c_int64

    # int64_t LoadRepository(char* repositoryName);
    lib.LoadRepository.argtypes = [ctypes.c_char_p]
    lib.LoadRepository.restype = ctypes.c_int64

    # int64_t RegisterNewVersion(char* versionName);
    lib.RegisterNewVersion.argtypes = [ctypes.c_char_p]
    lib.RegisterNewVersion.restype = ctypes.c_int64

    # int64_t LoadVersion(char* versionName);
    lib.LoadVersion.argtypes = [ctypes.c_char_p]
    lib.LoadVersion.restype = ctypes.c_int64

    # int64_t LoadAndSetPreviousVersion(char* previousVersionName);
    lib.LoadAndSetPreviousVersion.argtypes = [ctypes.c_char_p]
    lib.LoadAndSetPreviousVersion.restype = ctypes.c_int64

    # int64_t GetVersionInfo(char** versionInfo);
    lib.GetVersionInfo.argtypes = [ctypes.POINTER(ctypes.c_char_p)]
    lib.GetVersionInfo.restype = ctypes.c_int64

    # int64_t WriteToVersion(int64_t* compression, StatCallback callback, char** compressionRate);
    lib.WriteToVersion.argtypes = [ctypes.POINTER(ctypes.c_int64),
                                   ctypes.CFUNCTYPE(None, ctypes.c_int64, ctypes.c_int64, ctypes.c_uint64),
                                   ctypes.POINTER(ctypes.c_char_p)]
    lib.WriteToVersion.restype = ctypes.c_int64

    # int64_t ReadFromVersion(int64_t overwriteExistingFiles, StatCallback callback);
    lib.ReadFromVersion.argtypes = [ctypes.c_int64,
                                    ctypes.CFUNCTYPE(None, ctypes.c_int64, ctypes.c_int64, ctypes.c_uint64)]
    lib.ReadFromVersion.restype = ctypes.c_int64

    # int64_t EstimateRead(int64_t overwriteExistingFiles);
    lib.EstimateRead.argtypes = [ctypes.c_int64]
    lib.EstimateRead.restype = ctypes.c_int64

    # --------------- Archive Functions ---------------

    # int64_t CreateArchive(char* name, char* folder);
    lib.CreateArchive.argtypes = [ctypes.c_char_p,
                                  ctypes.c_char_p]
    lib.CreateArchive.restype = ctypes.c_int64

    # int64_t AddNewGroup(char* groupName, char* pathPrefix, StatCallback callback);
    lib.AddNewGroup.argtypes = [ctypes.c_char_p,
                                ctypes.c_char_p,
                                ctypes.CFUNCTYPE(None, ctypes.c_int64, ctypes.c_int64, ctypes.c_uint64)]
    lib.AddNewGroup.restype = ctypes.c_int64

    # int64_t LoadArchive(char* folder, archiveName **C.char);
    lib.LoadArchive.argtypes = [ctypes.c_char_p, ctypes.POINTER(ctypes.c_char_p)]
    lib.LoadArchive.restype = ctypes.c_int64

    # int64_t ReadArchiveGroup(char* groupName);
    lib.ReadArchiveGroup.argtypes = [ctypes.c_char_p]
    lib.ReadArchiveGroup.restype = ctypes.c_int64

    # int64_t ReadArchive(ReadCallback keyCallback, ReadCallback valueCallback, StatCallback callback);
    lib.ReadArchive.argtypes = [ctypes.CFUNCTYPE(ctypes.c_char_p),
                                ctypes.CFUNCTYPE(ctypes.c_char_p),
                                ctypes.CFUNCTYPE(None, ctypes.c_int64, ctypes.c_int64, ctypes.c_uint64)]
    lib.ReadArchive.restype = ctypes.c_int64

    # int64_t CloseArchive(char** compressionRate);
    lib.CloseArchive.argtypes = [ctypes.POINTER(ctypes.c_char_p)]
    lib.CloseArchive.restype = ctypes.c_int64

    # --------------- Helper Functions ---------------

    # void UpdateParameter(fileDivider int64_t, totalByteMarker uint64_t);
    lib.UpdateParameter.argtypes = [ctypes.c_int64, ctypes.c_uint64]
    lib.UpdateParameter.restype = None

    # void StreamArrayFromDLL(WriteCallback callback);
    lib.StreamArrayFromDLL.argtypes = [ctypes.CFUNCTYPE(None, ctypes.c_char_p)]
    lib.StreamArrayFromDLL.restype = None

    # void StreamArrayToDLL(ReadCallback callback);
    lib.StreamArrayToDLL.argtypes = [ctypes.CFUNCTYPE(ctypes.c_char_p)]
    lib.StreamArrayToDLL.restype = None

    # void ArrayExtendToDLL(ReadCallback callback);
    lib.ArrayExtendToDLL.argtypes = [ctypes.CFUNCTYPE(ctypes.c_char_p)]
    lib.ArrayExtendToDLL.restype = None

    # char* GetError();
    lib.GetError.argtypes = []
    lib.GetError.restype = ctypes.POINTER(ctypes.c_char_p)

    return lib


_LIB: ctypes.CDLL | None = None

def _get_lib() -> ctypes.CDLL:
    global _LIB
    if _LIB is None:
        _LIB = _load_lib()
    return _LIB

class BlobSession:
    def __init__(self, message_amount: int = 20, bytes_to_read_until_next_message: int = 128 * 1024 * 1024):
        self._lib  = _get_lib()
        self._ENCODING = "UTF-8"
        self._MESSAGE_AMOUNT = message_amount
        self._BYTES_MARKER = bytes_to_read_until_next_message
        self.__update_stats(message_amount, bytes_to_read_until_next_message)

        self.__compression_mapping = {
            "VERY LOW": 0,
            "LOW": 3,
            "MIDDLE": 6,
            "HIGH": 9,
            "VERY HIGH": 12,
        }
        self.__STANDARD_COMPRESSION = self.__compression_mapping["MIDDLE"]

    def set_standard_compression_level(self, map_string: str) -> BlobSession:
        if map_string not in self.__compression_mapping:
            raise RuntimeError(f"Compression level '{map_string}' not supported.")
        self.__STANDARD_COMPRESSION = self.__compression_mapping[map_string]
        return self

    # --------------- Direct Access Functions ---------------

    def open_for_writing(self, path: str, compression_level: int | None = None):
        """
        Opens the blobber.dll for writing to a blob file.

        :param path: the path of the blob file
        :param compression_level: the zstd compression level that should be used
        """
        if compression_level is None:
            compression_level = self.__STANDARD_COMPRESSION
        comp_level_val = ctypes.c_int64(compression_level)
        success = self._lib.BlobOpen(None, path.encode(self._ENCODING), ctypes.byref(comp_level_val))
        if not success:
            raise RuntimeError(self.__read_error())

    def open_for_reading(self, path: str):
        """
        Opens the blobber.dll for reading from a blob file.

        :param path: the path of the blob file
        """
        success = self._lib.BlobOpen(path.encode(self._ENCODING), None, None)
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
        file_ts_c = ctypes.c_int64(file_ts)

        hash_ptr = ctypes.c_char_p(None if hash_string is None else hash_string.encode(self._ENCODING))
        file_changed = ctypes.c_int64(0)

        success = self._lib.BlobCompress(path.encode(self._ENCODING),
                                         ctypes.byref(file_length_c),
                                         ctypes.byref(file_position_c),
                                         ctypes.byref(file_ts_c),
                                         ctypes.byref(hash_ptr),
                                         ctypes.byref(file_changed))
        if not success:
            raise RuntimeError(self.__read_error())

        return (None if file_changed.value == -1 else bool(file_changed.value),
                file_length_c.value, file_position_c.value, file_ts_c.value,
                hash_ptr.value.decode(self._ENCODING))

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

        success = self._lib.BlobDecompress(path.encode(self._ENCODING),
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
        statistics_ptr = ctypes.c_char_p(None)

        success = self._lib.BlobCloseWithStatistics(ctypes.byref(statistics_ptr))
        if not success:
            raise RuntimeError(self.__read_error())

        return statistics_ptr.value.decode(self._ENCODING)

    # --------------- Version Management Functions ---------------

    def open_overview(self, overview_path: str) -> list[str]:
        """
        Opens the general overview file that holds all the names of the repositories.

        :param overview_path: the path to the folder with the overview
        :return: the list of repo names in the overview
        """
        success = self._lib.LoadOverview(overview_path.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())
        return self.__read_array()

    def close_overview(self):
        """
        Closes the general overview and all underlying still opened repos and versions.
        """
        success = self._lib.CloseOverview()
        if not success:
            raise RuntimeError(self.__read_error())

    def new_repo(self, repo_name: str) -> list[str]:
        """
        Tries to create a new repository under the given name. If the name is taken, the function will raise
        a RuntimeError with the message containing the reason.

        :param repo_name: the name of the repository
        :return: the list of versions contained in the repository
        """
        success = self._lib.RegisterNewRepository(repo_name.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())
        return []

    def load_repo(self, repo_name: str) -> list[str]:
        """
        Tries to load a new repository from disk by the given name. If the name is not part of the repo list,
        the function will raise a RuntimeError with the message containing the reason.

        :param repo_name: the name of the repository
        :return: the list of versions contained in the repository
        """
        success = self._lib.LoadRepository(repo_name.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())
        return self.__read_array()

    def create_new_version(self, version_name, glob_commands: list[str]) -> list[str]:
        """
        Tries to create a version with the given name and glob commands.
        Will raise an exception if the name is already taken.

        :param version_name: name of the new version to create
        :param glob_commands: the glob paths to find the files with
        :return: the list of actual files in the version
        """
        self.__new_version(version_name)
        statistics, files = self.__write_to_version(glob_commands)
        print(f"Compressed files to {statistics} of size.")
        return files

    def new_version_from_old(self,
                             version_name: str,
                             old_version_name: str,
                             glob_commands: list[str]) -> list[str]:
        """
        Tries to create a version with the given name and glob commands comparing the file changes against the
        specified older version.
        Will raise an exception if the name is already taken or the old version does not exist.

        :param version_name: name of the new version to create
        :param old_version_name: name of the version to compare the file meta-data to
        :param glob_commands: the glob paths to find the files with
        :return: the list of actual files in the version
        """
        self.__new_version(version_name)
        self.__set_previous_version(old_version_name)
        statistics, files = self.__write_to_version(glob_commands)
        print(f"Compressed files to {statistics} of size.")
        return files

    def __new_version(self, version_name: str):
        """
        Tries to create a version with the given name. Will throw an error if the name is already taken.

        :param version_name: the name of the new version
        """
        success = self._lib.RegisterNewVersion(version_name.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())

    def load_version(self, version_name: str) -> list[str]:
        """
        Tries to load a version with the given name. If the name is not yet taken, the function will throw an error.

        :param version_name: the name of the version to load
        """
        success = self._lib.LoadVersion(version_name.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())
        return self.__read_array()

    def __set_previous_version(self, version_name: str):
        """
        Sets a previous version name for the currently loaded version.

        :param version_name: the previous version name to set
        """
        success = self._lib.LoadAndSetPreviousVersion(version_name.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())

    def get_version_info(self) -> str:
        version_info_ptr = ctypes.c_char_p(None)
        success = self._lib.GetVersionInfo(ctypes.byref(version_info_ptr))
        if not success:
            raise RuntimeError(self.__read_error())
        return version_info_ptr.value.decode(self._ENCODING)

    def __write_to_version(self, glob_commands: list[str], compression_level: int | None = None) -> tuple[str, list[str]]:
        """
        Writes all files specified by the glob commands to the current version.
        The function will print periodic information about its progress.

        :param glob_commands: the glob commands to execute to find the files desired to be stored in version
        :param compression_level: the desired compression level for the resulting blob
        :return: a tuple containing the compression rate (str) and list of files in the version (list[str])
        """
        if compression_level is None:
            compression_level = self.__STANDARD_COMPRESSION
        comp_level_val = ctypes.c_int64(compression_level)

        statistics_ptr = ctypes.c_char_p(None)

        print("Transferring path information.")
        self.__write_array([])
        for glob_cmd in glob_commands:
            self.__extend_array([val for val in glob.glob(glob_cmd, recursive=True, include_hidden=True)])

        cb_statistics = self.get_stat_func()

        print("Creating new Version.")
        success = self._lib.WriteToVersion(ctypes.byref(comp_level_val),
                                           cb_statistics,
                                           ctypes.byref(statistics_ptr))
        if not success:
            raise RuntimeError(self.__read_error())

        files_saved = self.__read_array()
        print(f"Finished saving {len(files_saved)} files to version.")

        return statistics_ptr.value.decode(self._ENCODING), files_saved

    def read_files_from_version(self, overwrite_existing_files: bool, paths: list[str]):
        """
        Reads files from the current version. The amount read can be limited with the paths list, which, if given,
        will limit the amount of files restored and if the ``overwrite_existing_files`` is set to true, makes
        an explicit check if the file is already present on disk before restoring it.
        The function will print periodic information about its progress.

        :param overwrite_existing_files: If files already on disk should be replaced by versioned file
        :param paths: a list of limiting paths to check against
        """
        self.__write_array(paths)
        overwrite = ctypes.c_int64(1 if overwrite_existing_files else 0)

        cb_statistics = self.get_stat_func()
        success = self._lib.ReadFromVersion(overwrite, cb_statistics)
        if not success:
            raise RuntimeError(self.__read_error())
        print("Finished restoring desired files.")

    def estimate_files_read(self, overwrite_existing_files: bool, paths: list[str]) -> list[str]:
        """
        Does a dry run of ``read_files_from_version()`` returning the files that would be restored.

        :param overwrite_existing_files: If files already on disk should be replaced by versioned file
        :param paths: a list of limiting paths to check against
        :return: a list of files that would be restored under the given conditions
        """
        self.__write_array(paths)
        overwrite = ctypes.c_int64(1 if overwrite_existing_files else 0)

        success = self._lib.EstimateRead(overwrite)
        if not success:
            raise RuntimeError(self.__read_error())

        return self.__read_array()

    # --------------- Archive Functions ---------------

    def create_archive(self, name: str, folder: str):
        """
        Creates a new archive with the given name at the given folder path.
        Opens a blob for writing into that folder.

        :param name: the name of the archive
        :param folder: the path to the folder where the archive should be created
        """
        success = self._lib.CreateArchive(name.encode(self._ENCODING),
                                          folder.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())

    def add_group_to_archive(self, group_name: str, path_prefix: str, glob_path: str):
        """
        Compresses the given paths into the open archive blob under the given group name,
        stripping path_prefix from each path to form the stored relative path.
        Raises a RuntimeError if any path does not start with path_prefix.

        :param group_name: the name of the group to add
        :param path_prefix: the prefix to strip from each path when storing
        :param glob_path: the glob command for paths to compress into the group
        """
        cb_statistics = self.get_stat_func()
        self.__write_array([path for path in glob.glob(glob_path, recursive=True, include_hidden=True)])
        success = self._lib.AddNewGroup(group_name.encode(self._ENCODING),
                                        path_prefix.encode(self._ENCODING),
                                        cb_statistics)
        print(f"Added Group '{group_name}' to archive.")
        if not success:
            raise RuntimeError(self.__read_error())

    def load_archive(self, folder: str) -> tuple[list[str], str]:
        """
        Loads an existing archive from the given folder, opening the blob for reading.

        :param folder: the path to the folder containing the archive
        :return: the list of group names contained in the archive and the archive name
        """
        name_ptr = ctypes.c_char_p(None)

        success = self._lib.LoadArchive(folder.encode(self._ENCODING), ctypes.byref(name_ptr))
        if not success:
            raise RuntimeError(self.__read_error())
        return self.__read_array(), name_ptr.value.decode(self._ENCODING)

    def read_archive_group_files(self, group_name: str) -> list[str]:
        """
        Reads all files for a specific group from the currently opened archive.

        :param group_name: the name of the group
        :return: the list of file paths contained in the group of the archive
        """
        success = self._lib.ReadArchiveGroup(group_name.encode(self._ENCODING))
        if not success:
            raise RuntimeError(self.__read_error())
        return self.__read_array()

    def read_archive(self, prefix_mapping: dict[str, str | None]):
        """
        Decompresses files from the open archive according to the given prefix mapping.
        Groups mapped to None are skipped entirely.
        For included groups the output path is: prefix_mapping[group_name] + relative_file_path.

        :param prefix_mapping: a dict of group_name -> target prefix string, or None to skip
        """
        buffer_array = [(ctypes.create_string_buffer(k.encode(self._ENCODING)),
                         ctypes.create_string_buffer(v.encode(self._ENCODING))
                        if v is not None else None)
                        for k, v in prefix_mapping.items()]

        key_index = 0

        read_callback = ctypes.CFUNCTYPE(ctypes.c_char_p)

        def distribute_keys():
            nonlocal key_index
            if key_index < len(buffer_array):
                key_index += 1
                buf, _ = buffer_array[key_index - 1]
                return ctypes.addressof(buf)
            return None

        def distribute_values():
            nonlocal key_index
            # This is ok, because distribute_keys() is always called before this function, if that contract is ever
            # not fulfilled this function might not work as expected.
            _, buf = buffer_array[key_index - 1]
            return ctypes.addressof(buf) if buf is not None else None

        cb_keys = read_callback(distribute_keys)
        cb_values = read_callback(distribute_values)
        cb_statistics = self.get_stat_func()

        success = self._lib.ReadArchive(cb_keys, cb_values, cb_statistics)
        print("Read all specified files from Archive.")
        if not success:
            raise RuntimeError(self.__read_error())

    def close_archive(self) -> str:
        """
        Saves the archive overview to disk and closes the blob.
        """
        statistics_ptr = ctypes.c_char_p(None)

        success = self._lib.CloseArchive(ctypes.byref(statistics_ptr))
        if not success:
            raise RuntimeError(self.__read_error())

        return statistics_ptr.value.decode(self._ENCODING)

    # --------------- Helper Functions ---------------

    def __update_stats(self, file_divider: int, process_byte_marker: int):
        self._lib.UpdateParameter(ctypes.c_int64(file_divider), ctypes.c_uint64(process_byte_marker))

    def __read_array(self) -> list[str]:
        callback = ctypes.CFUNCTYPE(None, ctypes.c_char_p)

        results = []
        def collect(s):
            results.append(s.decode(self._ENCODING))

        cb_collect = callback(collect)
        self._lib.StreamArrayFromDLL(cb_collect)

        return results

    def __write_array(self, strings: list[str]) -> None:
        callback = ctypes.CFUNCTYPE(ctypes.c_char_p)

        results = [ctypes.create_string_buffer(val.encode(self._ENCODING)) for val in strings]
        index = 0

        def distribute():
            nonlocal index
            if index < len(results):
                index += 1
                return ctypes.addressof(results[index - 1])
            return None

        cb_distribute = callback(distribute)
        self._lib.StreamArrayToDLL(cb_distribute)

    def __extend_array(self, strings: list[str]) -> None:
        callback = ctypes.CFUNCTYPE(ctypes.c_char_p)

        results = [ctypes.create_string_buffer(val.encode(self._ENCODING)) for val in strings]
        index = 0

        def distribute():
            nonlocal index
            if index < len(results):
                index += 1
                return ctypes.addressof(results[index - 1])
            return None

        cb_distribute = callback(distribute)
        self._lib.ArrayExtendToDLL(cb_distribute)

    def __read_error(self) -> str:
        txt_ptr = self._lib.GetError()
        result = ctypes.string_at(txt_ptr).decode(self._ENCODING)
        return result

    @staticmethod
    def get_stat_func():
        callback = ctypes.CFUNCTYPE(None, ctypes.c_int64, ctypes.c_int64, ctypes.c_uint64)

        def stat_printer(processed, actual_files_written, bytes_written):
            print(f"Processed {processed} paths, with a total of {actual_files_written} files ({bytes_written:_}B) compressed.")

        return callback(stat_printer)
