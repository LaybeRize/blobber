import glob
import os
import shutil
from pathlib import Path
from loader import BlobSession
from data_generator import generate_random_files, tree_compare_same

if __name__ == '__main__':
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
    for file in glob.glob(data_dir + f"{os.sep}**", recursive=True, include_hidden=True):
        status, length, pos, _, _ = session.read_file_to_blob(file)
        if status is not None:
            files.append((file, length, pos))
    print(session.close_blob_file_with_stats())
    print("Compressed", len(files), "files")
    session.open_for_reading(blob_path)
    for file_path, length, position in files:
        session.read_file_from_blob(result_dir + file_path.removeprefix(data_dir), length, position)
    session.close_blob_file()
    print("Finished File Functions")
    if tree_compare_same(data_dir, result_dir):
        print("Data 1 to 1 reproduced")
    else:
        print("Failed to verify that data was correctly reproduced")