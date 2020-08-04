# Changelog

## [Unreleased]

## [v2.0.0-rc1] - 2020-04-29

## [v2.0.0-beta3] - 2020-04-23

### Features

* Use Proxy and Router images in Controller env vars
* Increase wait time for Router IP
* Refactor for parallel reconciliation

### Bugs

* Update go-sdk module with WaitForLoadBalancer fix
* Fix CR errors

## [v2.0.0-beta2] - 2020-04-06

### Features

* Add retries to ioFog Controller client
* Add IsSupportedCustomResource
* Add Proxy service to ControlPlaneSpec
* Add CR helper functions to iofog pkg
* Refactor Kog to ControlPlane and make more optional fields in API type
* Add RouterImage to Kog spec

## [v2.0.0-beta] - 2020-03-12

### Features

* Upgrade go-sdk to v2
* Make PVC creation optional
* Add PV for Controller sqlite db

## [v2.0.0-alpha] - 2020-03-10

### Features

* Replace env var with API call to Controller for default Router
* Add port manager env vars
* Add Skupper loadbalancer IP and router ports to Controller env vars
* Remove connectors
* Add PortManagerImage to Kog ControlPlane
* Add env vars for Port Manager
* Add readiness probe to Port Manager
* Deploy Port Manager
* Deploy Skupper Router

### Bugs

* Consolidate usage of iofog client and reorganize controller reconciliation
* Removes all references to Connector
  
[Unreleased]: https://github.com/eclipse-iofog/iofog-operator/compare/v2.0.0-beta3..HEAD
[v2.0.0-beta2]: https://github.com/eclipse-iofog/iofog-operator/compare/v2.0.0-beta2..v2.0.0-beta3
[v2.0.0-beta2]: https://github.com/eclipse-iofog/iofog-operator/compare/v2.0.0-beta..v2.0.0-beta2
[v2.0.0-beta]: https://github.com/eclipse-iofog/iofog-operator/compare/v2.0.0-alpha..v2.0.0-beta
[v2.0.0-alpha]: https://github.com/eclipse-iofog/iofog-operator/tree/v2.0.0-alpha