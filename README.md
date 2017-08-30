# pgsql-novips [![CircleCI](https://circleci.com/gh/gocardless/pgsql-novips.svg?style=svg&circle-token=38c8f4dc817216aa6a02b3bf67435fe2f1d72189)](https://circleci.com/gh/gocardless/pgsql-novips)

https://paper.dropbox.com/doc/Postgres-Clustering-V2-d9N8n4cWuXZPeyTdeEpXw

## PGBouncer config

We use lib/pq to connect to PGBouncer over the unix socket. Unfortunately lib/pq
has issues when first establishing a connection to PGBouncer as it attempts to
set the configuration parameters `extra_float_digits`, which PGBouncer doesn't
recognise, and therefore will reject the connection.

To avoid this, make sure all configuration templates include the following:

```
[pgbouncer]
...

# Connecting using the golang lib/pq wrapper requires that we ignore
# the 'extra_float_digits' startup parameter, otherwise PGBouncer will
# close the connection.
#
# https://github.com/lib/pq/issues/475
ignore_startup_parameters = extra_float_digits
```

## CircleCI

We build a custom Docker image for CircleCI builds that is hosted at
gocardless/pgsql-novips-circleci on Docker Hub. The Dockerfile lives at
`.circleci/Dockerfile`, and there is a make target to build and push the image.

To publish a new version of the Docker image, run:

```bash
make publish-circleci-dockerfile
```
