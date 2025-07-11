/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util_test

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-vsphere/apis/v1beta1"
	vmwarev1 "sigs.k8s.io/cluster-api-provider-vsphere/apis/vmware/v1beta1"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/util"
)

func Test_GetMachinePreferredIPAddress(t *testing.T) {
	testCases := []struct {
		name        string
		machine     *infrav1.VSphereMachine
		ipAddr      string
		expectedErr error
	}{
		{
			name: "single IPv4 address, no preferred CIDR",
			machine: &infrav1.VSphereMachine{
				Status: infrav1.VSphereMachineStatus{
					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "192.168.0.1",
						},
					},
				},
			},
			ipAddr:      "192.168.0.1",
			expectedErr: nil,
		},
		{
			name: "single IPv6 address, no preferred CIDR",
			machine: &infrav1.VSphereMachine{
				Status: infrav1.VSphereMachineStatus{
					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "fdf3:35b5:9dad:6e09::0001",
						},
					},
				},
			},
			ipAddr:      "fdf3:35b5:9dad:6e09::0001",
			expectedErr: nil,
		},
		{
			name: "multiple IPv4 addresses, only 1 internal, no preferred CIDR",
			machine: &infrav1.VSphereMachine{
				Status: infrav1.VSphereMachineStatus{
					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "192.168.0.1",
						},
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "1.1.1.1",
						},
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "2.2.2.2",
						},
					},
				},
			},
			ipAddr:      "192.168.0.1",
			expectedErr: nil,
		},
		{
			name: "multiple IPv4 addresses, preferred CIDR set to v4",
			machine: &infrav1.VSphereMachine{
				Spec: infrav1.VSphereMachineSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							PreferredAPIServerCIDR: "192.168.0.0/16",
						},
					},
				},
				Status: infrav1.VSphereMachineStatus{
					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "192.168.0.1",
						},
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "172.17.0.1",
						},
					},
				},
			},
			ipAddr:      "192.168.0.1",
			expectedErr: nil,
		},
		{
			name: "multiple IPv4 and IPv6 addresses, preferred CIDR set to v4",
			machine: &infrav1.VSphereMachine{
				Spec: infrav1.VSphereMachineSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							PreferredAPIServerCIDR: "192.168.0.0/16",
						},
					},
				},
				Status: infrav1.VSphereMachineStatus{
					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "192.168.0.1",
						},
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "fdf3:35b5:9dad:6e09::0001",
						},
					},
				},
			},
			ipAddr:      "192.168.0.1",
			expectedErr: nil,
		},
		{
			name: "multiple IPv4 and IPv6 addresses, preferred CIDR set to v6",
			machine: &infrav1.VSphereMachine{
				Spec: infrav1.VSphereMachineSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							PreferredAPIServerCIDR: "fdf3:35b5:9dad:6e09::/64",
						},
					},
				},
				Status: infrav1.VSphereMachineStatus{

					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "192.168.0.1",
						},
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "fdf3:35b5:9dad:6e09::0001",
						},
					},
				},
			},
			ipAddr:      "fdf3:35b5:9dad:6e09::0001",
			expectedErr: nil,
		},
		{
			name: "no addresses found",
			machine: &infrav1.VSphereMachine{
				Spec: infrav1.VSphereMachineSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							PreferredAPIServerCIDR: "fdf3:35b5:9dad:6e09::/64",
						},
					},
				},
				Status: infrav1.VSphereMachineStatus{
					Addresses: []clusterv1beta1.MachineAddress{},
				},
			},
			ipAddr:      "",
			expectedErr: util.ErrNoMachineIPAddr,
		},
		{
			name: "no addresses found with preferred CIDR",
			machine: &infrav1.VSphereMachine{
				Spec: infrav1.VSphereMachineSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							PreferredAPIServerCIDR: "192.168.0.0/16",
						},
					},
				},
				Status: infrav1.VSphereMachineStatus{

					Addresses: []clusterv1beta1.MachineAddress{
						{
							Type:    clusterv1beta1.MachineExternalIP,
							Address: "10.0.0.1",
						},
					},
				},
			},
			ipAddr:      "",
			expectedErr: util.ErrNoMachineIPAddr,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ipAddr, err := util.GetMachinePreferredIPAddress(tc.machine)
			if err != tc.expectedErr {
				t.Logf("expected err: %q", tc.expectedErr)
				t.Logf("actual err: %q", err)
				t.Errorf("unexpected error")
			}

			if ipAddr != tc.ipAddr {
				t.Logf("expected IP addr: %q", tc.ipAddr)
				t.Logf("actual IP addr: %q", ipAddr)
				t.Error("unexpected IP addr from machine context")
			}
		})
	}
}

func Test_GetMachineMetadata(t *testing.T) {
	testCases := []struct {
		name            string
		machine         *infrav1.VSphereVM
		networkStatuses []infrav1.NetworkStatus
		ipamState       map[string]infrav1.NetworkDeviceSpec
		expected        string
	}{
		{
			name: "dhcp4",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: false
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: false
      accept-ra: false
`,
		},
		{
			name: "dhcp4+deviceName",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
									DeviceName:  "ens192",
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: false
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "ens192"
      wakeonlan: true
      dhcp4: true
      dhcp6: false
      accept-ra: false
`,
		},
		{
			name: "dhcp6",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP6:       true,
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: false
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
`,
		},
		{
			name: "dhcp4+dhcp6",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
									DHCP6:       true,
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: true
      accept-ra: true
`,
		},
		{
			name: "dhcp4+dhcp6+dhcpOverrides",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
									DHCP4Overrides: &infrav1.DHCPOverrides{
										Hostname:     toStringPtr("hal"),
										RouteMetric:  toIntPtr(12345),
										SendHostname: toBoolPtr(true),
										UseDNS:       toBoolPtr(true),
										UseDomains:   toStringPtr("true"),
										UseHostname:  toBoolPtr(true),
										UseMTU:       toBoolPtr(true),
										UseNTP:       toBoolPtr(true),
										UseRoutes:    toStringPtr("route"),
									},
									DHCP6: true,
									DHCP6Overrides: &infrav1.DHCPOverrides{
										Hostname:     toStringPtr("hal"),
										RouteMetric:  toIntPtr(12345),
										SendHostname: toBoolPtr(true),
										UseDNS:       toBoolPtr(true),
										UseDomains:   toStringPtr("true"),
										UseHostname:  toBoolPtr(true),
										UseMTU:       toBoolPtr(true),
										UseNTP:       toBoolPtr(true),
										UseRoutes:    toStringPtr("route"),
									},
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: true
      accept-ra: true
      dhcp4-overrides:
        hostname: "hal"
        route-metric: 12345
        send-hostname: true
        use-dns: true
        use-domains: true
        use-hostname: true
        use-mtu: true
        use-ntp: true
        use-routes: "route"
      dhcp6-overrides:
        hostname: "hal"
        route-metric: 12345
        send-hostname: true
        use-dns: true
        use-domains: true
        use-hostname: true
        use-mtu: true
        use-ntp: true
        use-routes: "route"
`,
		},
		{
			name: "dhcp4+dhcp6+noDhcpOverrides",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName:    "network1",
									MACAddr:        "00:00:00:00:00",
									DHCP4:          true,
									DHCP4Overrides: nil,
									DHCP6:          true,
									DHCP6Overrides: nil,
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: true
      accept-ra: true
`,
		},
		{
			name: "static4+dhcp6",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP6:       true,
									IPAddrs:     []string{"192.168.4.21"},
									Gateway4:    "192.168.4.1",
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
      addresses:
      - "192.168.4.21"
      gateway4: "192.168.4.1"
`,
		},
		{
			name: "static4+dhcp6+static-routes",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP6:       true,
									IPAddrs:     []string{"192.168.4.21"},
									Gateway4:    "192.168.4.1",
								},
							},
							Routes: []infrav1.NetworkRouteSpec{
								{
									To:     "192.168.5.1/24",
									Via:    "192.168.4.254",
									Metric: 3,
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
      addresses:
      - "192.168.4.21"
      gateway4: "192.168.4.1"
  routes:
  - to: "192.168.5.1/24"
    via: "192.168.4.254"
    metric: 3
`,
		},
		{
			name: "2nets",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
									Routes: []infrav1.NetworkRouteSpec{
										{
											To:     "192.168.5.1/24",
											Via:    "192.168.4.254",
											Metric: 3,
										},
									},
								},
								{
									NetworkName: "network12",
									MACAddr:     "00:00:00:00:01",
									DHCP6:       true,
									MTU:         mtu(100),
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: false
      accept-ra: false
      routes:
      - to: "192.168.5.1/24"
        via: "192.168.4.254"
        metric: 3
    id1:
      match:
        macaddress: "00:00:00:00:01"
      set-name: "eth1"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
      mtu: 100
`,
		},
		{
			name: "2nets-static+dhcp",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName:   "network1",
									MACAddr:       "00:00:00:00:00",
									IPAddrs:       []string{"192.168.4.21"},
									Gateway4:      "192.168.4.1",
									MTU:           mtu(0),
									Nameservers:   []string{"1.1.1.1"},
									SearchDomains: []string{"vmware.ci"},
								},
								{
									NetworkName:   "network12",
									MACAddr:       "00:00:00:00:01",
									DHCP6:         true,
									SearchDomains: []string{"vmware6.ci"},
								},
							},
						},
					},
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: false
      dhcp6: false
      accept-ra: false
      addresses:
      - "192.168.4.21"
      gateway4: "192.168.4.1"
      nameservers:
        addresses:
        - "1.1.1.1"
        search:
        - "vmware.ci"
    id1:
      match:
        macaddress: "00:00:00:00:01"
      set-name: "eth1"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
      nameservers:
        search:
        - "vmware6.ci"
`,
		},
		{
			name: "2nets+network-statuses",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
								},
								{
									NetworkName: "network12",
									MACAddr:     "00:00:00:00:01",
									DHCP6:       true,
								},
							},
						},
					},
				},
			},
			networkStatuses: []infrav1.NetworkStatus{
				{MACAddr: "00:00:00:00:ab"},
				{MACAddr: "00:00:00:00:cd"},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:ab"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: false
      accept-ra: false
    id1:
      match:
        macaddress: "00:00:00:00:cd"
      set-name: "eth1"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
`,
		},
		{
			name: "ipam state is used to render metadata",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
								},
								{
									NetworkName: "network2",
									MACAddr:     "00:00:00:00:01",
									DHCP4:       true,
								},
								{
									NetworkName: "network3",
									MACAddr:     "00:00:00:00:02",
								},
							},
						},
					},
				},
			},
			ipamState: map[string]infrav1.NetworkDeviceSpec{
				"00:00:00:00:00": {
					IPAddrs: []string{
						"10.10.50.50/24",
					},
					Gateway4: "10.10.50.1",
				},
				"00:00:00:00:02": {
					IPAddrs: []string{
						"fe80::3/64",
					},
					Gateway6: "fe80::1",
				},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: false
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:00"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: false
      dhcp6: false
      accept-ra: false
      addresses:
      - "10.10.50.50/24"
      gateway4: "10.10.50.1"
    id1:
      match:
        macaddress: "00:00:00:00:01"
      set-name: "eth1"
      wakeonlan: true
      dhcp4: true
      dhcp6: false
      accept-ra: false
    id2:
      match:
        macaddress: "00:00:00:00:02"
      set-name: "eth2"
      wakeonlan: true
      dhcp4: false
      dhcp6: false
      accept-ra: false
      addresses:
      - "fe80::3/64"
      gateway6: "fe80::1"
`,
		},
		{
			name: "more-network-statuses-than-spec-devices",
			machine: &infrav1.VSphereVM{
				Spec: infrav1.VSphereVMSpec{
					VirtualMachineCloneSpec: infrav1.VirtualMachineCloneSpec{
						Network: infrav1.NetworkSpec{
							Devices: []infrav1.NetworkDeviceSpec{
								{
									NetworkName: "network1",
									MACAddr:     "00:00:00:00:00",
									DHCP4:       true,
								},
								{
									NetworkName: "network12",
									MACAddr:     "00:00:00:00:01",
									DHCP6:       true,
								},
							},
						},
					},
				},
			},
			networkStatuses: []infrav1.NetworkStatus{
				{MACAddr: "00:00:00:00:ab"},
				{MACAddr: "00:00:00:00:cd"},
				{MACAddr: "00:00:00:00:ef"},
			},
			expected: `
instance-id: "test-vm"
local-hostname: "test-vm"
wait-on-network:
  ipv4: true
  ipv6: true
network:
  version: 2
  ethernets:
    id0:
      match:
        macaddress: "00:00:00:00:ab"
      set-name: "eth0"
      wakeonlan: true
      dhcp4: true
      dhcp6: false
      accept-ra: false
    id1:
      match:
        macaddress: "00:00:00:00:cd"
      set-name: "eth1"
      wakeonlan: true
      dhcp4: false
      dhcp6: true
      accept-ra: true
    id2:
      match:
        macaddress: "00:00:00:00:ef"
      set-name: "eth2"
      wakeonlan: true
      dhcp4: false
      dhcp6: false
      accept-ra: false
`,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.machine.Name = tc.name
			actVal, err := util.GetMachineMetadata("test-vm", *tc.machine, tc.ipamState, tc.networkStatuses...)
			if err != nil {
				t.Fatal(err)
			}

			if string(actVal) != tc.expected {
				t.Logf("actual metadata value: %s", actVal)
				t.Logf("expected metadata value: %s", tc.expected)
				t.Error("unexpected metadata value")
			}
		})
	}
}

func TestConvertProviderIDToUUID(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testCases := []struct {
		name         string
		providerID   *string
		expectedUUID string
	}{
		{
			name:         "nil providerID",
			providerID:   nil,
			expectedUUID: "",
		},
		{
			name:         "empty providerID",
			providerID:   toStringPtr(""),
			expectedUUID: "",
		},
		{
			name:         "invalid providerID",
			providerID:   toStringPtr("1234"),
			expectedUUID: "",
		},
		{
			name:         "missing prefix",
			providerID:   toStringPtr("12345678-1234-1234-1234-123456789abc"),
			expectedUUID: "",
		},
		{
			name:         "valid providerID",
			providerID:   toStringPtr("vsphere://12345678-1234-1234-1234-123456789abc"),
			expectedUUID: "12345678-1234-1234-1234-123456789abc",
		},
		{
			name:         "mixed case",
			providerID:   toStringPtr("vsphere://12345678-1234-1234-1234-123456789AbC"),
			expectedUUID: "12345678-1234-1234-1234-123456789AbC",
		},
		{
			name:         "invalid hex chars",
			providerID:   toStringPtr("vsphere://12345678-1234-1234-1234-123456789abg"),
			expectedUUID: "",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(*testing.T) {
			actualUUID := util.ConvertProviderIDToUUID(tc.providerID)
			g.Expect(actualUUID).To(gomega.Equal(tc.expectedUUID))
		})
	}
}

func TestConvertUUIDtoProviderID(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testCases := []struct {
		name               string
		uuid               string
		expectedProviderID string
	}{
		{
			name:               "empty uuid",
			uuid:               "",
			expectedProviderID: "",
		},
		{
			name:               "invalid uuid",
			uuid:               "1234",
			expectedProviderID: "",
		},
		{
			name:               "valid uuid",
			uuid:               "12345678-1234-1234-1234-123456789abc",
			expectedProviderID: "vsphere://12345678-1234-1234-1234-123456789abc",
		},
		{
			name:               "mixed case",
			uuid:               "12345678-1234-1234-1234-123456789AbC",
			expectedProviderID: "vsphere://12345678-1234-1234-1234-123456789AbC",
		},
		{
			name:               "invalid hex chars",
			uuid:               "12345678-1234-1234-1234-123456789abg",
			expectedProviderID: "",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(*testing.T) {
			actualProviderID := util.ConvertUUIDToProviderID(tc.uuid)
			g.Expect(actualProviderID).To(gomega.Equal(tc.expectedProviderID))
		})
	}
}

func Test_MachinesAsString(t *testing.T) {
	tests := []struct {
		machines     []*clusterv1.Machine
		errorMessage string
	}{
		{
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "m1-ns"}},
			},
			errorMessage: "m1-ns/m1",
		},
		{
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "m1-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2", Namespace: "m2-ns"}},
			},
			errorMessage: "m1-ns/m1 and m2-ns/m2",
		},
		{
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "m1-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2", Namespace: "m2-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3", Namespace: "m3-ns"}},
			},
			errorMessage: "m1-ns/m1, m2-ns/m2 and m3-ns/m3",
		},
	}

	for _, tt := range tests {
		g := gomega.NewWithT(t)
		msg := util.MachinesAsString(tt.machines)
		g.Expect(msg).To(gomega.Equal(tt.errorMessage))
	}
}

func Test_GetVSphereClusterFromVSphereMachine(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = vmwarev1.AddToScheme(scheme)

	ns := "util-test"

	incorrectMachine := &vmwarev1.VSphereMachine{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
	}
	machine := &vmwarev1.VSphereMachine{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "foo"},
			Name:      "foo-machine-1",
			Namespace: ns,
		},
	}
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: ns,
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &clusterv1.ContractVersionedObjectReference{
				Name: "foo-abcdef", // auto generated name
			},
		},
	}
	vsphereCluster := &vmwarev1.VSphereCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-abcdef",
			Namespace: ns,
		},
	}

	testCases := []struct {
		name         string
		initObjects  []client.Object
		inputMachine *vmwarev1.VSphereMachine
		hasError     bool
	}{
		{
			name:         "for machine without CAPI cluster name label",
			hasError:     true,
			inputMachine: incorrectMachine,
		},
		{
			name:         "for non-existent CAPI cluster",
			hasError:     true,
			inputMachine: machine,
		},
		{
			name:         "for non-existent VSphereCluster",
			hasError:     true,
			inputMachine: machine,
			initObjects:  []client.Object{cluster},
		},
		{
			name:         "for non-existent VSphereCluster",
			inputMachine: machine,
			initObjects:  []client.Object{cluster, vsphereCluster},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.initObjects...).Build()
			_, err := util.GetVSphereClusterFromVMwareMachine(context.Background(), client, tt.inputMachine)
			if tt.hasError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
		})
	}
}

func mtu(i int64) *int64 {
	if i == 0 {
		return nil
	}
	return &i
}

func toStringPtr(s string) *string {
	return &s
}

func toBoolPtr(b bool) *bool {
	return &b
}

func toIntPtr(i int) *int {
	return &i
}
