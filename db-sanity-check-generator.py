import click
from tqdm import tqdm
import random
import os

MAX_FILE_SIZE = 300 * 1000 * 1000  # 300 MB


@click.command()
# The place to store the output
@click.argument('output_path', type=click.Path(exists=False))
# the population density map
@click.argument('table_name', type=str)
@click.option(
    '--count',
    '-c',
    'count',
    type=int,
    default=50 * 1000 * 1000
)
def main(
    output_path: str,
    table_name: str,
    count: int,
):
    if not os.path.isdir(output_path):
        raise Exception(f"Output path has to be a directory")

    idx = 0
    f = open(os.path.join(output_path, f"part-{idx}.sql"), "w")

    is_first_line = True

    for i in tqdm(
        range(count),
        total=count,
        desc="generate"
    ):

        if random.randint(0, 1) == 0:

            # don't add comma on the first line
            if is_first_line:
                size = f.write(
                    f"INSERT INTO {table_name} (id, payload) VALUES\n"
                )
                is_first_line = False
            else:
                f.write(",\n")

            size += f.write(
                f"({i},'{random.randbytes(32).hex()}')"
            )

            if size >= MAX_FILE_SIZE:
                f.write("\nON CONFLICT (id) DO NOTHING")
                f.close()

                idx += 1
                f = open(os.path.join(output_path, f"part-{idx}.sql"), "w")

                is_first_line = True

    f.write("\nON CONFLICT (id) DO NOTHING")
    f.close()


if __name__ == '__main__':
    main()
