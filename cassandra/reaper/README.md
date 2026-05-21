[[_TOC_]]

# Cassandra Reaper

## Repository structure

* `./docs` - directory with API description.
* `.gitlab-ci.yml` - the CI/CD pipelines configuration.
* `./build.sh` - the entrypoint for build job, it starts docker image build.
* `./description.yaml` - descibes buld sructure of Cassandra Reaper docker image.
* `./Dockerfile` - the Dockerfile for Cassandra Reaper docker image.
* `./run.sh` - the run.sh script used as an entry point.


## Evergreen strategy

To keep the component up to date, the following activities should be performed regularly:

* Vulnerabilities fixing.
* Bug-fixing, improvement and feature implementation.
