import glob
import os
import random
import shutil
from pathlib import Path
from loader import BlobSession
from data_generator import generate_random_files, tree_compare_same

def main():
    test_raw_functions()
    test_version_functions()


def test_raw_functions():
    data_dir = f"{Path(__file__).parent}{os.sep}data"
    result_dir = data_dir + "2"
    blob_path = f"{Path(__file__).parent}{os.sep}blob.blob"
    shutil.rmtree(data_dir, ignore_errors=True)
    shutil.rmtree(result_dir, ignore_errors=True)
    generate_random_files(
        base_folder=data_dir,
        file_sizes=[
            64 * 1024,         # 64 KiB
            5 * 1024 * 1024,   #  5 MiB
            19 * 1024 * 1024,  # 19 MiB
        ],
        num_files=30,
    )

    files: list[tuple[str, int, int]] = []
    session = BlobSession()
    session.open_for_writing(blob_path)
    bytes_processed = 0
    for file in glob.glob(data_dir + f"{os.sep}**", recursive=True, include_hidden=True):
        status, length, pos, _, _ = session.read_file_to_blob(file)
        if status is not None:
            bytes_processed += length
            files.append((file, length, pos))
    print(f'{bytes_processed:_}B processed')
    print("Compression Rate:", session.close_blob_file_with_stats())
    print("Compressed", len(files), "files")
    session.open_for_reading(blob_path)
    for file_path, length, position in files:
        session.read_file_from_blob(result_dir + file_path.removeprefix(data_dir), length, position)
    session.close_blob_file()
    print("--- Finished File Functions ---")
    if not tree_compare_same(data_dir, result_dir):
        print("+++ Failed to verify that data was correctly reproduced +++")
        exit(-1)

    print("--- Data 1 to 1 reproduced ---")


def test_version_functions():
    ver_dir = f"{Path(__file__).parent}{os.sep}versioning"
    data_dir = f"{Path(__file__).parent}{os.sep}data"
    rename_dir = data_dir + "_old"
    shutil.rmtree(ver_dir, ignore_errors=True)
    shutil.rmtree(data_dir, ignore_errors=True)
    shutil.rmtree(rename_dir, ignore_errors=True)
    #
    print("--- Creating Data Tree ---")
    file_amt = 40
    generate_random_files(
        base_folder=data_dir,
        file_sizes=[
            64 * 1024,         # 64 KiB
            5 * 1024 * 1024,   #  5 MiB
            19 * 1024 * 1024,  # 19 MiB
            50 * 1024 * 1024,  # 50 MiB
        ],
        num_files=file_amt,
    )
    #
    print("--- Testing Session Management ---")
    #
    session = BlobSession()
    session.open_overview(ver_dir)
    session.new_repo("test_repo")
    session.close_overview()
    if not os.path.exists(ver_dir + f"{os.sep}general.overview"):
        print("+++ Failed to locate the appropriate overview file expected +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.repo")) != 1:
        print("+++ Failed to locate the exact amount of repos (1) expected +++")
        exit(-1)
    session.open_overview(ver_dir)
    session.load_repo("test_repo")
    #
    print("--- Creating first Version ---")
    #
    files = session.create_new_version("version1", [data_dir + f"{os.sep}**"])
    if len(files) != file_amt:
        print("+++ Not all files were properly saved to the version 1 +++")
        exit(-1)
    session.close_overview()
    if len(glob.glob(ver_dir + f"{os.sep}*.version")) != 1:
        print("+++ Failed to locate the exact amount of versions (1) expected +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.blob")) != 1:
        print("+++ Failed to locate the exact amount of version blobs (1) expected +++")
        exit(-1)

    print("--- Testing version restore capabilities ---")

    session.open_overview(ver_dir)
    session.load_repo("test_repo")
    session.load_version("version1")

    files = session.estimate_files_read(False, [])
    if len(files) != 0:
        print("+++ Files from version 1 are mistakenly restored +++")
        exit(-1)

    os.replace(data_dir, rename_dir)
    files = session.estimate_files_read(False, [])
    if len(files) != file_amt:
        print("+++ Files from version 1 are missing from the restore preview restored +++")
        exit(-1)

    print("--- Restoring version 1 ---")

    session.read_files_from_version(False, [])

    if not tree_compare_same(rename_dir, data_dir):
        print("+++ Failed to verify that data from version 1 was correctly reproduced +++")
        exit(-1)

    print("--- Restored version 1 successful ---")

    shutil.rmtree(rename_dir, ignore_errors=True)
    files = list(filter(lambda x: os.path.isfile(x), glob.glob(data_dir + f"{os.sep}**", recursive=True, include_hidden=True)))
    changed_files = random.sample(files, 4)
    for file in changed_files:
        with open(file, "w", encoding="UTF-8") as f:
            f.write("AHHHHHHHHHHHHHHHHHHHHHHHH TEST TEST TEST ETSET TEST TEST TEST "
                    "AHHHHHHHHHHHHHHHHHHHHHHHHAHHHHHHHHHHHHHHHHHHHHHHHHAHHHHHHHHHHH"
                    "HHHHHHHHHHHHHAHHHHHHHHHHHHHHHHHHHHHHHHAHHHHHHHHHHHHHHHHHHHHHHHH")

    print("--- Creating version 2 ---")

    files = session.new_version_from_old("version2","version1", [data_dir + f"{os.sep}**"])
    if len(files) != file_amt:
        print("+++ Did not capture all files in version 2 +++")
        exit(-1)

    print("--- Restoring version 2 ---")

    os.replace(data_dir, rename_dir)
    session.read_files_from_version(False, [])

    if not tree_compare_same(rename_dir, data_dir):
        print("+++ Failed to verify that data from version 2 was correctly reproduced +++")
        exit(-1)

    session.close_overview()
    if len(glob.glob(ver_dir + f"{os.sep}*.version")) != 2:
        print("+++ Failed to locate the exact amount of versions (2) expected +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.blob")) != 2:
        print("+++ Failed to locate the exact amount of version blobs (2) expected +++")
        exit(-1)

    print("--- Versioning test was successful ---")


if __name__ == '__main__':
    main()