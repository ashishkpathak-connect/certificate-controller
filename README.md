# certificate-controller
## Overview
The certificate-controller aims to solve generation of Self Signed TLS Certificates by automation. It adds new custom resource certificates(Kind: Certificate) to Kubernetes.

## Description
The certificate-controller manages creation/update/delete of custom resource certificates(kind: Certificate) on kubernetes and creates/updates/deletes a TLS type Secret containing Self Signed Certificate and Private Key in an automated way. The Secret can then be used by applications to secure their HTTP endpoints. It's a self-service way of requesting TLS certificates for application developers. 
It was scaffolded using a modern framework kubebuilder and uses Ginkgo for [tests](internal/controller/certificate_controller_test.go)/[tests](internal/certs/selfsignedcert_test.go).

## Features
- Supports Creation/Update/Deletion of TLS type Secret.
- Supports Status as per [metav1.Condition](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition).
- Supports Finalizers to ensure deletion of secret in case of Certificate delete or update.
- Supports Emitting events for different [phases](api/v1/certificate_types.go) of Secret lifecycle. Visible in `kubectl describe certificates` output.
- Supports Custom [metrics](internal/controller/certificate_controller.go) certStatus and secretEvents of type Counter to track certificates(Available/Degraded) and secrets(create/delete) respectively.
- Supports custom column "STATUS" in `kubectl get certificates` output. 

## Working Details

### Creation
- Create a certificates custom resource(kind: Certificate) including .spec.dnsName, .spec.validity and .spec.secretRef.name
All these 3 fields are mandatory and validations are included in openAPIV3Schema for:
   - .spec.dnsName and .spec.secretRef.name should be a valid DNS subdomain name.
   - .spec.validity should be a string containing a sequence of one or more digits followed by d.
- The Kubernetes APIServer validates the above 3 fields and creates a certificates custom resource upon successful validation.
- This is detected as an event by certificate-controller and processes it. 
- The controller first checks if the secret specified by .spec.secretRef.name exists in the cluster in the desired namespace. If so, match contents of the secret(actual) vs spec of certificates(desired). If they match, check if .status.condition.type of certificates is "Available". If "Available", do nothing else update to "Available". If the contents of actuals vs desired do not match then process as below.
- Update .status.condition.type of certificates to Progressing, emit "CreatingSecret", add finalizer, try creating the resource(i.e., Self Signed Certificate, Private Key and then a TLS type Secret containing the Certificate and Key). If the resource creation is successful increments the secretEvents counter. In the subsequent loop, the .status.condition.type is updated to "Available", emits "CreatedSecret" event and increments certStatus counter to "Available". If the resource creation fails, it's processed as below.
- Another attempt(total:2) is made to create resource and if it's not successful, then the .status.condition.type is marked as "Degraded", emits FailedCreatingSecret event and increments certStatus counter to "Degraded". After this controller tries to reconcile every 5 second to create resource.

### Deletion
- Deleting a certificates custom resource(kind: Certificate) is detected by the controller.
- It does not gets deleted immediately as there is a finalizer present which helps controller to clean up the secret.
- The controller checks if the secret is present in the desired namespace and deleted it. Event "DeletedSecret" is emitted and secretEvents counter is incremented.
- The finalizer is removed from the custom resource and Kubernetes deletes it.

### Update

#### Updating DNSName/Validity/both
> Note: This would lead to Update of Existing secret as its not a change to .spec.secretRef.name
- When an existing certificates custom resource(kind: Certificate) .spec.dnsName or .spec.validity is updated, it is detected by the controller.
- The controller checks if the secret specified by .spec.secretRef.name exists in the desired namespace in Kubernetes cluster. If so, check secret contents(i.e., actual) and match with .spec of certificates(i.e., desired). 
- If its not a match, update .status.condition.type of certificates to "Progressing", emit event "CreatingSecret" and update the existing secret. In the subsequent loop, update .status.condition.type to "Available", emit "CreatedSecret" event and increment the counter certStatus. 
- If its a match do nothing.

#### Updating SecretRefName
> Note: This would lead to Recreation of secret with the new name as specified by .spec.secretRef.name i.e., old secret is removed and new is created. This helps in achieving memory efficiency. Also finalizer is updated.
- When an existing certificates custom resource(kind: Certificate) .spec.secretRef.name is updated, it is detected by the controller.
- The controller checks if the secret specified by .spec.secretRef.name of certificates exist. if not present does the below.
- Checks if the old secret specified by .status.secretName is present in the cluster. If so, attempts to delete it. If unable to delete or not present, then it just proceeds to below.
- Resets the .status.condition of certificates so taht it goes through the below creation flow.
- Update .status.condition.type of certificates to Progressing, emit "CreatingSecret",removes old finalizer referencing .status.secretName, adds new finalizer referencing .spec.secretRef.name, try creating the resource(i.e., Self Signed Certificate, Private Key and then a TLS type Secret containing the Certificate and Key). If the resource creation is successful increments the secretEvents counter. In the subsequent loop, the .status.condition.type is updated to "Available", emits "CreatedSecret" event and increments certStatus counter to "Available". If the resource creation fails, it's processed as below.
- Another attempt(total:2) is made to create resource and if it's not successful, then the .status.condition.type is marked as "Degraded", emits FailedCreatingSecret event and increments certStatus counter to "Degraded". After this controller tries to reconcile every 5 second to create resource.

## Metrics
- Metrics endpoint is at :8443/metrics of certificate-controller and is protected by authentication/authorization [role](config/rbac/metrics_auth_role.yaml)/[rolebinding](config/rbac/metrics_auth_role_binding.yaml). Monitoring systems(example: Prometheus) may need rolebinding of [role](config/rbac/metrics_reader_role.yaml) to their serviceAccount to be able to scrape metrics from controller.
- Custom metrics certStatus and secretEvents of type Counter are exposed.
- A /readyz and /healthz endpoints are exposed at :8081 of certificate-controller which can be used to probe health check by endpoint monitoring systems like blackbox-exporter.


## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To build the certificate-controller

```sh
make
```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/certificate-controller:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

or

```sh
make docker-build IMG=certificate-controller:tag
```

**Deploy the certificate-controller to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/certificate-controller:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### Run Tests

```sh
make test
```

or

```sh
go test ./internal/... -v
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/certificate-controller:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/certificate-controller/<tag or branch>/dist/install.yaml
```

[metav1.Condition]: <https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition>