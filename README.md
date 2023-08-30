# route-to-contour-httpproxy

A Kubernetes controller for converting Openshift HAProxy Route to Contour HTTPProxy

## Description

Currently, the project consists of a single controller which watches `route` and `httpproxy` resources.
The controller tries to create/update an `httpproxy` that matches the corresponding `route(s)` in functionalities, e.g. tls
termination, port mapping, etc.
