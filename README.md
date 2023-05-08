# Route-To-Contour-Httpproxy Helm Chart

## Usage

[Helm](https://helm.sh) must be installed to use the charts. Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

```shell
helm repo add route-to-contour-httpproxy https://snapp-incubator.github.io/route-to-contour-httpproxy
```

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages. You can then run `helm search repo
route-to-contour-httpproxy` to see the charts.

To install the route-to-contour-httpproxy chart:

```shell
helm install my-release route-to-contour-httpproxy/route-to-contour-httpproxy
```

To uninstall the chart:

```shell
helm delete my-release
```
