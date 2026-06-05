import glob
import os
import random
import string
from pathlib import Path

def generate_random_files(
        base_folder: str,
        file_sizes: list[int],
        num_files: int = 30,
        max_depth: int = 5,
        min_files_per_folder: int = 2,
        max_files_per_folder: int = 4,
):
    """
    Generate random files in a nested folder structure.

    :param base_folder: root directory to generate files in
    :param file_sizes: list of possible file sizes in bytes to pick from randomly
    :param num_files: total number of files to generate (default: 20)
    :param max_depth: maximum folder nesting depth (default: 5)
    :param min_files_per_folder: minimum of files generated per folder (default: 2)
    :param max_files_per_folder: maximum of files generated per folder (default: 4)
    """
    base_folder = base_folder.removesuffix(os.sep)
    os.makedirs(base_folder, exist_ok=True)

    def random_name(length=8) -> str:
        return "".join(random.choices(string.ascii_lowercase, k=length))

    def random_path(current_path: str) -> str:
        """Generate a random nested path up to max_depth levels deep."""
        while True:
            if current_path.removeprefix(base_folder).count(os.sep) >= max_depth:
                remove_folders = random.randint(min(2, max_depth), max_depth)
                for _ in range(remove_folders):
                    current_path = str(Path(current_path).parent)
                return os.path.join(current_path, random_name())
            if random.randint(0, 2):
                return os.path.join(current_path, random_name())
            if current_path != base_folder:
                current_path = str(Path(current_path).parent)


    # Track which folders have at least one file
    folders_with_files: dict[str, int] = {}
    created_folders: list[str] = []
    current_counter = 0

    def write_file(folder_path: str) -> None:
        os.makedirs(folder_path, exist_ok=True)
        if folder_path not in created_folders:
            created_folders.append(folder_path)
        size = random.choice(file_sizes)
        filename = random_name() + ".bin"
        path = os.path.join(folder_path, filename)
        with open(path, "wb") as f:
            f.write(os.urandom(size))

        if folder_path in folders_with_files:
            folders_with_files[folder_path] += 1
        else:
            folders_with_files[folder_path] = 1

    current_folder_path = base_folder
    # Generate num_files files in random locations
    while True:
        for _ in range(min_files_per_folder):
            write_file(current_folder_path)
            current_counter += 1
            if current_counter >= num_files:
                return
        for _ in range(max_files_per_folder - min_files_per_folder):
            if random.randint(0, 1):
                break
            write_file(current_folder_path)
            current_counter += 1
            if current_counter >= num_files:
                return
        current_folder_path = random_path(current_folder_path)

def tree_compare_same(folder_one: str, folder_two: str) -> bool:
    folder_one = folder_one.removesuffix(os.sep)
    folder_two = folder_two.removesuffix(os.sep)
    files = glob.glob(folder_one + os.sep + "**", recursive=True, include_hidden=True)
    for file in files:
        alt_path = folder_two + file.removeprefix(folder_one)
        if not os.path.exists(alt_path):
            return False
        if os.path.isfile(file) and not files_compare_same(file, alt_path):
            return False
    return True

def files_compare_same(file_one: str, file_two: str) -> bool:
    chunk_size = 4 * 1024 * 1024
    with open(file_one, "rb") as f1:
        with open(file_two, "rb") as f2:
            while chunk := f1.read(chunk_size):
                if chunk != f2.read(chunk_size):
                    return False
    return True
