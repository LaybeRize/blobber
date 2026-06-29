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
    test_archive_functions()


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

    print("--- Testing Session Management ---")

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
    session.set_repo_glob_list(["a", "b", "c"])

    print("--- Creating first Version ---")

    session.create_new_version("version1", [data_dir + f"{os.sep}**"])
    if session.current_version_name != "version1":
        print("+++ local current version name wrong +++")
        exit(-1)

    _, files = session.get_version_info()
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
    if session.load_repo("test_repo") != ["version1"]:
        print("+++ Failed to correctly retrieve version list +++")
        exit(-1)
    session.load_version("version1")
    if ["a", "b", "c"] != session.get_repo_glob_list():
        print("+++ Failed to correctly retrieve glob list +++")
        exit(-1)
    stats, _ = session.get_version_info()
    print(stats)

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

    session.new_version_from_old("version2","version1", [data_dir + f"{os.sep}**"])
    if session.current_version_name != "version2":
        print("+++ local current version name wrong +++")
        exit(-1)

    _, files = session.get_version_info()
    if len(files) != file_amt:
        print("+++ Did not capture all files in version 2 +++")
        exit(-1)

    print("--- Restoring version 2 ---")

    os.replace(data_dir, rename_dir)
    session.read_files_from_version(False, [])

    if not tree_compare_same(rename_dir, data_dir):
        print("+++ Failed to verify that data from version 2 was correctly reproduced +++")
        exit(-1)

    session.close_version()
    if session.current_version_name is not None:
        print("+++ Failed to close Version +++")
        exit(-1)
    session.close_repo()
    if session.current_repo_name is not None:
        print("+++ Failed to close Version +++")
        exit(-1)

    session.close_overview()
    if len(glob.glob(ver_dir + f"{os.sep}*.version")) != 2:
        print("+++ Failed to locate the exact amount of versions (2) expected +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.blob")) != 2:
        print("+++ Failed to locate the exact amount of version blobs (2) expected +++")
        exit(-1)

    print("--- Testing Deletes ---")
    session.open_overview(ver_dir)
    new_file = ver_dir + f"{os.sep}other_file.txt"
    with open(new_file, "w") as file:
        file.write("Test")
    amt = session.clean_overview()
    if amt != 1 or os.path.exists(new_file):
        print("+++ Failed to delete files not in the overview +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.version")) != 2:
        print("+++ Failed to locate the exact amount of versions (2) expected after cleanup +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.blob")) != 2:
        print("+++ Failed to locate the exact amount of version blobs (2) expected after cleanup +++")
        exit(-1)
    session.load_repo("test_repo")
    session.delete_version("version1")
    if len(glob.glob(ver_dir + f"{os.sep}*.version")) != 1:
        print("+++ Failed to locate the exact amount of versions (1) expected after version delete +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.blob")) != 2:
        print("+++ Failed to locate the exact amount of version blobs (2) expected after version delete +++")
        exit(-1)
    session.close_repo()
    session.delete_repo("test_repo")
    if len(glob.glob(ver_dir + f"{os.sep}*.version")) != 0:
        print("+++ Located version files after delete +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.blob")) != 0:
        print("+++ Located blob files after delete +++")
        exit(-1)
    if len(glob.glob(ver_dir + f"{os.sep}*.repo")) != 0:
        print("+++ Located repo files after delete +++")
        exit(-1)
    session.close_overview()
    if len(session.open_overview(ver_dir)) != 0:
        print("+++ Failed to correctly delete repositories +++")
        exit(-1)

    print("--- Versioning test was successful ---")


def test_archive_functions():
    archive_dir = f"{Path(__file__).parent}{os.sep}archive_folder"
    data_dir = f"{Path(__file__).parent}{os.sep}data"
    target_dir = data_dir + "_target"
    sub_dir_one = f"{os.sep}test1"
    sub_dir_two = f"{os.sep}test2"
    shutil.rmtree(archive_dir, ignore_errors=True)
    shutil.rmtree(data_dir, ignore_errors=True)
    shutil.rmtree(target_dir, ignore_errors=True)
    print("--- Creating Data Tree ---")
    file_amt = 20
    generate_random_files(
        base_folder=data_dir+sub_dir_one,
        file_sizes=[
            64 * 1024,         # 64 KiB
            5 * 1024 * 1024,   #  5 MiB
            19 * 1024 * 1024,  # 19 MiB
            50 * 1024 * 1024,  # 50 MiB
        ],
        num_files=file_amt,
    )
    generate_random_files(
        base_folder=data_dir+sub_dir_two,
        file_sizes=[
            64 * 1024,         # 64 KiB
            5 * 1024 * 1024,   #  5 MiB
            19 * 1024 * 1024,  # 19 MiB
            50 * 1024 * 1024,  # 50 MiB
        ],
        num_files=file_amt,
    )
    print("--- Testing Archive Creation ---")
    session = BlobSession()
    session.create_archive("Test Archive", "TheCREATOR", archive_dir)
    session.add_group_to_archive("Group 1", data_dir+sub_dir_one, [data_dir+sub_dir_one + f"{os.sep}**"])
    session.add_group_to_archive("Group 2", data_dir+sub_dir_two, [data_dir+sub_dir_two + f"{os.sep}**"])
    session.close_archive()
    if not os.path.exists(archive_dir + f"{os.sep}archive.overview"):
        print("+++ Failed to locate the appropriate overview file expected +++")
        exit(-1)
    if not os.path.exists(archive_dir + f"{os.sep}archive.blob"):
        print("+++ Failed to locate the appropriate blob file expected +++")
        exit(-1)

    print("--- Testing Archive Restoration ---")
    groups, name, creator = session.load_archive(archive_dir)
    if groups != ["Group 1", "Group 2"]:
        print("+++ Unexpected list of groups +++")
        exit(-1)
    if name != "Test Archive" or creator != "TheCREATOR":
        print("+++ Unexpected name or creator for Archive +++")
        exit(-1)

    file_list = session.read_archive_group_files("Group 1")
    for file_name in file_list:
        if not os.path.exists(os.path.join(data_dir+sub_dir_one, file_name.removeprefix(os.sep))):
            print(os.path.join(data_dir+sub_dir_one, file_name.removeprefix(os.sep)))
            print("+++ Unexpected file in Group Archive list +++")
            exit(-1)
    session.read_archive({"Group 1": target_dir+sub_dir_two, "Group 2": target_dir+sub_dir_one})
    session.close_archive()

    print("--- Comparing restored Archive Data ---")

    if not tree_compare_same(data_dir+sub_dir_one, target_dir+sub_dir_two):
        print("+++ Failed to verify that data from 'Group 1' was correctly reproduced +++")
        exit(-1)

    if not tree_compare_same(data_dir+sub_dir_two, target_dir+sub_dir_one):
        print("+++ Failed to verify that data from 'Group 2' was correctly reproduced +++")
        exit(-1)

    print("--- Archive test was successful ---")


if __name__ == '__main__':
    main()