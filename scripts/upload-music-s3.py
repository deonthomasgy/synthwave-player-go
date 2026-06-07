#!/usr/bin/env python3
"""Sync local music library to MinIO S3 music bucket."""

import os
import sys
import time
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
from pathlib import Path
from threading import local

import boto3
from botocore.client import Config
from botocore.exceptions import ClientError

MUSIC_DIR = os.environ.get("MUSIC_DIR", "/mnt/share/640GB/Music_Deon")
BUCKET = os.environ.get("MINIO_BUCKET", "music")
ENDPOINT = os.environ.get("MINIO_ENDPOINT", "http://truenas-scale.sphinx-butterfly.ts.net:9000")
ACCESS_KEY = os.environ.get("MINIO_ACCESS_KEY", "thomas")
SECRET_KEY = os.environ.get("MINIO_SECRET_KEY", "")
WORKERS = int(os.environ.get("UPLOAD_WORKERS", "8"))
SKIP_NAMES = {".ds_store", "thumbs.db"}
SKIP_SUFFIXES = {".part"}

_thread_local = local()


def log(msg: str) -> None:
    try:
        print(msg, flush=True)
    except BrokenPipeError:
        pass


def get_client():
    return boto3.client(
        "s3",
        endpoint_url=ENDPOINT,
        aws_access_key_id=ACCESS_KEY,
        aws_secret_access_key=SECRET_KEY,
        region_name="us-east-1",
        config=Config(signature_version="s3v4", s3={"addressing_style": "path"}),
    )


def client_for_thread():
    if not hasattr(_thread_local, "client"):
        _thread_local.client = get_client()
    return _thread_local.client


def iter_files(root: str):
    root_path = Path(root)
    for path in root_path.rglob("*"):
        if not path.is_file():
            continue
        name = path.name.lower()
        if name in SKIP_NAMES or path.suffix.lower() in SKIP_SUFFIXES:
            continue
        yield path


def object_exists(client, key: str, size: int) -> bool:
    try:
        head = client.head_object(Bucket=BUCKET, Key=key)
        return head.get("ContentLength") == size
    except ClientError as exc:
        code = exc.response.get("Error", {}).get("Code", "")
        if code in {"404", "NoSuchKey", "NotFound"}:
            return False
        raise


def upload_file(local_path: Path, music_root: Path):
    client = client_for_thread()
    key = str(local_path.relative_to(music_root)).replace("\\", "/")
    size = local_path.stat().st_size
    if object_exists(client, key, size):
        return "skipped", key, size
    client.upload_file(str(local_path), BUCKET, key)
    return "uploaded", key, size


def main():
    if not SECRET_KEY:
        log("MINIO_SECRET_KEY is required")
        return 1
    if not os.path.isdir(MUSIC_DIR):
        log(f"Music directory not found: {MUSIC_DIR}")
        return 1

    music_root = Path(MUSIC_DIR)
    uploaded = skipped = failed = processed = 0
    bytes_uploaded = 0
    start = time.time()
    max_pending = WORKERS * 4

    log(f"Syncing files from {MUSIC_DIR} to s3://{BUCKET}/")
    log(f"Endpoint: {ENDPOINT}  workers: {WORKERS}")

    with ThreadPoolExecutor(max_workers=WORKERS) as pool:
        pending = set()
        for path in iter_files(MUSIC_DIR):
            pending.add(pool.submit(upload_file, path, music_root))
            if len(pending) < max_pending:
                continue

            done, pending = wait(pending, return_when=FIRST_COMPLETED)
            for future in done:
                processed += 1
                try:
                    status, _key, size = future.result()
                    if status == "uploaded":
                        uploaded += 1
                        bytes_uploaded += size
                    else:
                        skipped += 1
                except Exception as exc:
                    failed += 1
                    log(f"FAIL: {exc}")

                if processed % 100 == 0:
                    elapsed = time.time() - start
                    log(
                        f"[{processed}] uploaded={uploaded} skipped={skipped} "
                        f"failed={failed} sent={bytes_uploaded / (1024**3):.2f}GB "
                        f"elapsed={elapsed/60:.1f}m"
                    )

        while pending:
            done, pending = wait(pending, return_when=FIRST_COMPLETED)
            for future in done:
                processed += 1
                try:
                    status, _key, size = future.result()
                    if status == "uploaded":
                        uploaded += 1
                        bytes_uploaded += size
                    else:
                        skipped += 1
                except Exception as exc:
                    failed += 1
                    log(f"FAIL: {exc}")

                if processed % 100 == 0:
                    elapsed = time.time() - start
                    log(
                        f"[{processed}] uploaded={uploaded} skipped={skipped} "
                        f"failed={failed} sent={bytes_uploaded / (1024**3):.2f}GB "
                        f"elapsed={elapsed/60:.1f}m"
                    )

    elapsed = time.time() - start
    log(
        f"Done. processed={processed} uploaded={uploaded} skipped={skipped} "
        f"failed={failed} sent={bytes_uploaded / (1024**3):.2f}GB "
        f"elapsed={elapsed/60:.1f}m"
    )
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
