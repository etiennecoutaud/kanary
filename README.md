# Kanary : Canary Release Operator for Kubernetes
[![toto](https://travis-ci.org/etiennecoutaud/kanary.svg?branch=master)](https://travis-ci.org/etiennecoutaud/kanary.svg?branch=master)
[![codecov](https://codecov.io/gh/etiennecoutaud/kanary/branch/master/graph/badge.svg)](https://codecov.io/gh/etiennecoutaud/kanary)
[![Go Report Master](https://goreportcard.com/badge/github.com/etiennecoutaud/kanary)](https://goreportcard.com/report/github.com/etiennecoutaud/kanary)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/etiennecoutaud/kanary/blob/master/LICENSE)


This Kubernetes operator aim to manage canary release deployment using HAProxy container as L4 (TCP) loadbalancer

## Installation
```
kubectl apply -f https://URL
```

## How it works
Kanary Operator will acting on *kanary* CRD and will perform:
* HAProxy loabalancer generation
* Manage configuration and update based on **weight loadbalancing**
* Reference pod endpoint directly for more efficiency
* Trigger automatic HAProxy rolling update when configuration changed for no service outage
* Manage route as Kubernetes service to access to HAProxy

Kanary Operator is manipulating Kubernetes ressources:
* Deployment
* Endpoint
* ConfigMap
* Service

A RBAC dedicated role is needed to provide the ability to perform all operation on theses ressources

## CustomRessourceDefinitions

## Architecture and example

## License
The work done has been licensed under Apache License 2.0. The license file can be found [here](LICENSE). You can find
out more about the license at [www.apache.org/licenses/LICENSE-2.0](//www.apache.org/licenses/LICENSE-2.0).