# Work In Progress...

# route-to-contour-httpproxy

A Kubernetes controller for converting Openshift HAProxy Route to Contour HTTPProxy

## Description

Currently, the project consists of a single controller which watches `route` and `httpproxy` resources.
The controller tries to create/update an `httpproxy` that matches the corresponding `route` in functionalities, e.g. tls
termination, port mapping, etc.

## Getting Started

You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for
testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever
cluster `kubectl cluster-info` shows).

### Running on the cluster

1. Build and push your image to the location specified by `IMG`:

    ```sh
    make docker-build docker-push IMG=<some-registry>/route-to-contour-httpproxy:tag
    ```

2. Deploy the controller to the cluster with the image specified by `IMG`:

    ```sh
    make deploy IMG=<some-registry>/route-to-contour-httpproxy:tag
    ```

### Undeploy controller

UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing

### How it works

This project aims to follow the
Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the
cluster.

### Running Tests

1. Make sure the sample config exists first.

```shell
ls -l hack/config.yaml
```

2. Run the tests defined inside Makefile. These tests run on envtest, so you don't need to connect to a working cluster.

```shell
make test
```

### Test It Out

1. Run the controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
