# ExternalDNS - Utho Webhook

ExternalDNS is a Kubernetes add-on for automatically managing
Domain Name System (DNS) records for Kubernetes services by using different DNS providers.


By default, Kubernetes manages DNS records internally,
but ExternalDNS takes this functionality a step further by delegating the management of DNS records to an external DNS
provider such as this one.


Therefore, the Utho webhook allows to manage your
Utho domains inside your kubernetes cluster with [ExternalDNS](https://github.com/kubernetes-sigs/external-dns).

To use ExternalDNS with Utho, you need your Utho API token of the account managing
your domains.

---

## Example Usage

For step-by-step examples on installing ExternalDNS with Utho integration, refer to the [examples directory](./example/README.md).
