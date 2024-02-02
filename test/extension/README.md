# CAPV test extension

Tilt example configuration

```yaml
provider_repos:
- "../cluster-api-provider-vsphere"
- "../cluster-api-provider-vsphere/test/extension"
enable_providers:
- core
- kubeadm-bootstrap
- kubeadm-control-plane
- vsphere-supervisor
- capv-test-extension
template_dirs:
  vsphere-supervisor:
  - ../cluster-api-provider-vsphere/templates/supervisor
debug:
  capv-test-extension:
    port: 36000
    continue: true
    profiler_port: 36001
    metrics_port: 36002
```
