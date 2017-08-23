# pgsql-novips [![CircleCI](https://circleci.com/gh/gocardless/pgsql-novips.svg?style=svg&circle-token=38c8f4dc817216aa6a02b3bf67435fe2f1d72189)](https://circleci.com/gh/gocardless/pgsql-novips)

https://paper.dropbox.com/doc/Postgres-Clustering-V2-d9N8n4cWuXZPeyTdeEpXw

## CircleCI

We build a custom Docker image for CircleCI builds that is hosted at
gocardless/pgsql-novips-circleci on Docker Hub. The Dockerfile lives at
`.circleci/Dockerfile`, and there is a make target to build and push the image.

To publish a new version of the Docker image, run:

```bash
make publish-circleci-dockerfile
```
