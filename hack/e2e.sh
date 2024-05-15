#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit  # exits immediately on any unexpected error (does not bypass traps)
set -o nounset  # will error if variables are used without first being defined
set -o pipefail # any non-zero exit code in a piped command causes the pipeline to fail with that code

export PATH=${PWD}/hack/tools/bin:${PATH}
REPO_ROOT=$(git rev-parse --show-toplevel)

# In CI, ARTIFACTS is set to a different directory. This stores the value of
# ARTIFACTS i1n ORIGINAL_ARTIFACTS and replaces ARTIFACTS by a temporary directory
# which gets cleaned up from credentials at the end of the test.
export ORIGINAL_ARTIFACTS=""
export ARTIFACTS="${ARTIFACTS:-${REPO_ROOT}/_artifacts}"
if [[ "${ARTIFACTS}" != "${REPO_ROOT}/_artifacts" ]]; then
  ORIGINAL_ARTIFACTS="${ARTIFACTS}"
  ARTIFACTS=$(mktemp -d)
fi

# shellcheck source=./hack/ensure-kubectl.sh
source "${REPO_ROOT}/hack/ensure-kubectl.sh"

on_exit() {
  # release IPClaim
  echo "Releasing IP claims"
  kubectl --kubeconfig="${KUBECONFIG}" delete "ipaddressclaim.ipam.cluster.x-k8s.io" "${CONTROL_PLANE_IPCLAIM_NAME}" || true
  kubectl --kubeconfig="${KUBECONFIG}" delete "ipaddressclaim.ipam.cluster.x-k8s.io" "${WORKLOAD_IPCLAIM_NAME}" || true

  # kill the VPN
  docker kill vpn

  # Cleanup VSPHERE_PASSWORD from temporary artifacts directory.
  if [[ "${ORIGINAL_ARTIFACTS}" != "" ]]; then
    # Delete non-text files from artifacts directory to not leak files accidentially
    find "${ARTIFACTS}" -type f -exec file --mime-type {} \; | grep -v -E -e "text/plain|text/xml|application/json|inode/x-empty" | while IFS= read -r line
    do
      file="$(echo "${line}" | cut -d ':' -f1)"
      mimetype="$(echo "${line}" | cut -d ':' -f2)"
      echo "Deleting file ${file} of type ${mimetype}"
      rm "${file}"
    done || true
    # Replace secret and base64 secret in all files.
    if [ -n "$VSPHERE_PASSWORD" ]; then
      grep -I -r -l -e "${VSPHERE_PASSWORD}" "${ARTIFACTS}" | while IFS= read -r file
      do
        echo "Cleaning up VSPHERE_PASSWORD from file ${file}"
        sed -i "s/${VSPHERE_PASSWORD}/REDACTED/g" "${file}"
      done || true
      VSPHERE_PASSWORD_B64=$(echo -n "${VSPHERE_PASSWORD}" | base64 --wrap=0)
      grep -I -r -l -e "${VSPHERE_PASSWORD_B64}" "${ARTIFACTS}" | while IFS= read -r file
      do
        echo "Cleaning up VSPHERE_PASSWORD_B64 from file ${file}"
        sed -i "s/${VSPHERE_PASSWORD_B64}/REDACTED/g" "${file}"
      done || true
    fi
    # Move all artifacts to the original artifacts location.
    mv "${ARTIFACTS}"/* "${ORIGINAL_ARTIFACTS}/"
  fi
}

trap on_exit EXIT

export VSPHERE_SERVER="${GOVC_URL}"
export VSPHERE_USERNAME="${GOVC_USERNAME}"
export VSPHERE_PASSWORD="${GOVC_PASSWORD}"
export VSPHERE_SSH_AUTHORIZED_KEY="${VM_SSH_PUB_KEY}"
export VSPHERE_SSH_PRIVATE_KEY="/root/ssh/.private-key/private-key"
export E2E_CONF_FILE="${REPO_ROOT}/test/e2e/config/vsphere-ci.yaml"
export ARTIFACTS="${ARTIFACTS:-${REPO_ROOT}/_artifacts}"
export DOCKER_IMAGE_TAR="/tmp/images/image.tar"
export GC_KIND="false"

# Run the vpn client in container
docker run --rm -d --name vpn -v "${HOME}/.openvpn/:${HOME}/.openvpn/" \
  -w "${HOME}/.openvpn/" --cap-add=NET_ADMIN --net=host --device=/dev/net/tun \
  gcr.io/k8s-staging-capi-vsphere/extra/openvpn:latest

# Tail the vpn logs
docker logs vpn

function kubectl_get_jsonpath() {
  local OBJECT_KIND="${1}"
  local OBJECT_NAME="${2}"
  local JSON_PATH="${3}"
  local n=0
  until [ $n -ge 30 ]; do
    OUTPUT=$(kubectl --kubeconfig="${KUBECONFIG}" get "${OBJECT_KIND}.ipam.cluster.x-k8s.io" "${OBJECT_NAME}" -o=jsonpath="${JSON_PATH}")
    if [[ "${OUTPUT}" != "" ]]; then
      break
    fi
    n=$((n + 1))
    sleep 1
  done

  if [[ "${OUTPUT}" == "" ]]; then
    echo "Received empty output getting ${JSON_PATH} from ${OBJECT_KIND}/${OBJECT_NAME}" 1>&2
    return 1
  else
    echo "${OUTPUT}"
    return 0
  fi
}

function claim_ip() {
  IPCLAIM_NAME="$1"
  export IPCLAIM_NAME
  envsubst < "${REPO_ROOT}/hack/ipclaim-template.yaml" | kubectl --kubeconfig="${KUBECONFIG}" create -f - 1>&2
  IPADDRESS_NAME=$(kubectl_get_jsonpath ipaddressclaim "${IPCLAIM_NAME}" '{@.status.addressRef.name}')
  kubectl --kubeconfig="${KUBECONFIG}" get "ipaddresses.ipam.cluster.x-k8s.io" "${IPADDRESS_NAME}" -o=jsonpath='{@.spec.address}'
}

export KUBECONFIG="/root/ipam-conf/capv-services.conf"

# Wait until the VPN connection is active and we are able to reach the ipam cluster
function wait_for_ipam_reachable() {
  local n=0
  until [ $n -ge 30 ]; do
    kubectl --kubeconfig="${KUBECONFIG}" --request-timeout=2s  get inclusterippools.ipam.cluster.x-k8s.io && RET=$? || RET=$?
    if [[ "$RET" -eq 0 ]]; then
      break
    fi
    n=$((n + 1))
    sleep 1
  done
  return "$RET"
}
wait_for_ipam_reachable

make envsubst

# Retrieve an IP to be used as the kube-vip IP
CONTROL_PLANE_IPCLAIM_NAME="ip-claim-$(openssl rand -hex 20)"
CONTROL_PLANE_ENDPOINT_IP=$(claim_ip "${CONTROL_PLANE_IPCLAIM_NAME}")
export CONTROL_PLANE_ENDPOINT_IP
echo "Acquired Control Plane IP: $CONTROL_PLANE_ENDPOINT_IP"

# Retrieve an IP to be used for the workload cluster in v1a3/v1a4 -> v1b1 upgrade tests
WORKLOAD_IPCLAIM_NAME="workload-ip-claim-$(openssl rand -hex 20)"
WORKLOAD_CONTROL_PLANE_ENDPOINT_IP=$(claim_ip "${WORKLOAD_IPCLAIM_NAME}")
export WORKLOAD_CONTROL_PLANE_ENDPOINT_IP
echo "Acquired Workload Cluster Control Plane IP: $WORKLOAD_CONTROL_PLANE_ENDPOINT_IP"

# Run e2e tests
make e2e
