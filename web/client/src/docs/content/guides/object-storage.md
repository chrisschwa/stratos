# Object Storage Buckets

Object storage holds unstructured data — backups, media, artifacts, static assets — in S3-style buckets rather than on a disk attached to a server. You'll find it under **Storage → Object storage** in the sidebar.

## Buckets

A **bucket** is a flat container for objects (files), each addressed by a key. Create a bucket, then upload objects into it. Because object storage is reached over an API rather than mounted like a volume, it suits data that many clients read and write independently, that needs to scale without provisioning disks, or that you want to serve directly.

## Working with objects

Within a bucket you upload, download, and delete objects. The store presents an S3-compatible surface, so existing S3 tooling and SDKs — pointed at the platform's object-storage endpoint with your credentials — work against your buckets without change.

## When to reach for it

Prefer object storage over a [block volume](/docs/guides/volumes) when the data is file-shaped rather than a disk: think image and video assets, log and backup archives, build artifacts, or anything a web front end serves directly. Reach for a volume instead when a single server needs a real filesystem it can mount and run a database or application on.
