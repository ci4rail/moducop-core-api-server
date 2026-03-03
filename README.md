# go-template

a go-template project

The project uses `make` to make your life easier. 

Whenever you need help regarding the available actions, just use the following command.

```bash
make help
```

## Bootstrap

When moving a go project you have to adjust the module path and docker image name.
Search for `ci4rail/go-template` and replace all matches with your new repository.

## Setup

To get your setup up and running the only thing you have to do is

```bash
make all
```

This will initialize a git repo, download the dependencies in the latest versions and install all needed tools.
If needed code generation will be triggered in this target as well.

## Test & lint

Run linting

```bash
make lint
```

Run tests

```bash
make test
```

## create multiarch docker image

Run docker build and push

```bash
make docker
```