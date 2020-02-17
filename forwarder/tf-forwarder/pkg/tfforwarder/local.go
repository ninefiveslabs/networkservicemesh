// Copyright 2019 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tfforwarder

import (
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"runtime"

	"github.com/networkservicemesh/networkservicemesh/controlplane/api/crossconnect"
	"github.com/networkservicemesh/networkservicemesh/forwarder/tf-forwarder/pkg/monitoring"
	"github.com/networkservicemesh/networkservicemesh/utils/fs"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"fmt"
	"strconv"

	"github.com/networkservicemesh/networkservicemesh/contrail/common"
	"github.com/networkservicemesh/networkservicemesh/contrail/contrail"
	log "github.com/networkservicemesh/networkservicemesh/contrail/logging"
)

type tfInterfaceData struct {
	VmiUuid string
	VmUuid  string
	VnUuid  string
}

func tf(containerNetns, containerIntfName, ns string) error {
	retfail := fmt.Errorf("error in tf()")
	log.Init("cnilog", 40000, 40000)
	fmt.Printf("tf(%s, %s)", containerNetns, containerIntfName)
	containerId := containerIntfName
	grepCmd := exec.Command("python", "/bin/tfapi.py", containerId, ns)
	grepOut, _ := grepCmd.StdoutPipe()
	grepCmd.Start()
	grepBytes, _ := ioutil.ReadAll(grepOut)
	grepCmd.Wait()
	fmt.Println(string(grepBytes))
	var tfInterface tfInterfaceData
	json.Unmarshal([]byte(string(grepBytes)), &tfInterface)
	fmt.Println("vmi", tfInterface.VmiUuid)
	fmt.Println("vm", tfInterface.VmUuid)
	containerUuid := tfInterface.VmUuid
	vmiUuid := tfInterface.VmiUuid
	vnUuid := tfInterface.VnUuid
	containerName := "containerName"
	updateAgent := true
	mtu := 1500
	fmt.Printf("contrailCni.VRouterInit2 containerId %s, containerUuid %s, vnUuid %s, vmiUuid %s \n", containerId, containerUuid, vnUuid, vmiUuid)
	vrouter, _ := contrailCni.VRouterInit2(containerId, containerUuid, vnUuid, vmiUuid)
	fmt.Printf("vrouter.PollUrl(/vm-cfg)\n")
	results, poll_err := vrouter.PollUrl("/vm-cfg")
	if poll_err != nil {
		fmt.Println("Error in polling VRouter (PollUrl vm-cfg)")
		return retfail
	}
	fmt.Println("PollUrl results:", results)
	fmt.Printf("cniIntf.CniIntfMethods(cniIntf.InitVEth(containerIntfName %s, containerId %s, containerUuid %s, containerNetns %s, mtu %d))\n", containerIntfName, containerId, containerUuid, containerNetns, mtu)
	intfMethods := cniIntf.CniIntfMethods(cniIntf.InitVEth(containerIntfName, containerId, containerUuid, containerNetns, mtu))
	err := intfMethods.Create()
	if err != nil {
		fmt.Printf("Error creating interface object. Error %+v\n", err)
		return retfail
	}
	fmt.Println(intfMethods.GetHostIfName())
	fmt.Printf("vrouter.Add(containerName %s, containerUuid %s, vnUuid %s, containerId %s, containerNetns %s, containerIntfName %s, intfMethods.GetHostIfName() %s, vmiUuid %s, updateAgent %s", containerName, containerUuid, vnUuid, containerId, containerNetns, containerIntfName, intfMethods.GetHostIfName(), vmiUuid, updateAgent)
	err = vrouter.Add(containerName, containerUuid, vnUuid, containerId, containerNetns, containerIntfName, intfMethods.GetHostIfName(), vmiUuid, updateAgent)
	if err != nil {
		fmt.Printf("Error in Add to VRouter. Error %+v\n", err)
		return retfail
	}
	containerIntfNames := make(map[string]string)
	containerIntfNames[vmiUuid] = containerIntfName
	vrouterResultMap := make(map[string]contrailCni.Result)
	vRouterResults, poll_err := vrouter.PollUrl("/vm")
	if poll_err != nil {
		fmt.Printf("Error in polling VRouter PollUrl (/vm)")
		return retfail
	}
	for _, vRouterResult := range *vRouterResults {
		vrouterResultMap[vRouterResult.VmiUuid] = vRouterResult
	}
	fmt.Printf("About to configure %d interfaces for container\n",
		len(containerIntfNames))
	for vmiUuid := range containerIntfNames {
		containerIntfName := containerIntfNames[vmiUuid]
		vRouterResult, ok := vrouterResultMap[vmiUuid]
		if ok == false {
			msg := fmt.Sprintf("VMI UUID %s does not exist in the Vrouter Result\n",
				vmiUuid)
			fmt.Printf(msg)
		}
		log.Infof("Working on VrouterResult - %+v  and Interface name - %s\n",
			vRouterResult, containerIntfName)
		typesResult := contrailCni.MakeCniResult(containerIntfName, &vRouterResult)
		fmt.Println(typesResult.IPs)
		fmt.Println(typesResult.Interfaces)

		intfMethods.Configure(vRouterResult.Mac, typesResult)

	}
	return nil
}

// handleLocalConnection either creates or deletes a local connection - same host
func handleLocalConnection(crossConnect *crossconnect.CrossConnect, connect bool) (map[string]monitoring.Device, error) {
	logrus.Info("local: connection type - local source/local destination")
	var devices map[string]monitoring.Device
	/* 1. Get the connection configuration */
	cfg, err := newConnectionConfig(crossConnect, cLOCAL)
	logrus.Info("local: connCfg", cfg.srcName, cfg.dstName, cfg.srcIP)
	if err != nil {
		logrus.Errorf("local: failed to get connection configuration - %v", err)
		return nil, err
	}
	if connect {
		/* 2. Create a connection */
		devices, err = createLocalConnection(cfg, crossConnect.Source.NetworkService)
		if err != nil {
			logrus.Errorf("local: failed to create connection - %v", err)
			devices = nil
		}
	} else {
		/* 3. Delete a connection */
		devices, err = deleteLocalConnection(cfg)
		if err != nil {
			logrus.Errorf("local: failed to delete connection - %v", err)
			devices = nil
		}
	}
	return devices, err
}

// createLocalConnection handles creating a local connection
func createLocalConnection(cfg *connectionConfig, ns string) (map[string]monitoring.Device, error) {
	logrus.Info("local: creating connection...")
	/* Lock the OS thread so we don't accidentally switch namespaces */
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	/* Get namespace handler - source */
	u64, err := strconv.ParseUint(cfg.srcNetNsInode, 10, 64)
	srcNsHandle, err := fs.ResolvePodNsByInode(u64)
	if err != nil {
		logrus.Errorf("local: failed to get source namespace handle - %v", err)
		return nil, err
	}
	uniqName := cfg.dstName[len(cfg.dstName)-3 : len(cfg.dstName)]
	err = tf(srcNsHandle, uniqName, ns)
	if err != nil {
		fmt.Println("Error in tf()")
		return nil, err

	}

	logrus.Infof("local: creation completed for devices - source: %s, destination: %s", cfg.srcName, cfg.dstName)
	srcDevice := monitoring.Device{Name: cfg.srcName, XconName: "SRC-" + cfg.id}
	dstDevice := monitoring.Device{Name: cfg.dstName, XconName: "DST-" + cfg.id}
	return map[string]monitoring.Device{cfg.srcNetNsInode: srcDevice, cfg.dstNetNsInode: dstDevice}, nil
}

// deleteLocalConnection handles deleting a local connection
func deleteLocalConnection(cfg *connectionConfig) (map[string]monitoring.Device, error) {
	logrus.Info("local: deleting connection...")
	/* Lock the OS thread so we don't accidentally switch namespaces */
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	/* Get namespace handler - source */
	srcNsHandle, err := fs.GetNsHandleFromInode(cfg.srcNetNsInode)
	if err != nil {
		logrus.Errorf("local: failed to get source namespace handle - %v", err)
		return nil, err
	}
	/* If successful, don't forget to close the handler upon exit */
	defer func() {
		if err = srcNsHandle.Close(); err != nil {
			logrus.Error("local: error when closing source handle: ", err)
		}
		logrus.Debug("local: closed source handle: ", srcNsHandle, cfg.srcNetNsInode)
	}()
	logrus.Debug("local: opened source handle: ", srcNsHandle, cfg.srcNetNsInode)

	/* Get namespace handler - destination */
	dstNsHandle, err := fs.GetNsHandleFromInode(cfg.dstNetNsInode)
	if err != nil {
		logrus.Errorf("local: failed to get destination namespace handle - %v", err)
		return nil, err
	}
	defer func() {
		if err = dstNsHandle.Close(); err != nil {
			logrus.Error("local: error when closing destination handle: ", err)
		}
		logrus.Debug("local: closed destination handle: ", dstNsHandle, cfg.dstNetNsInode)
	}()
	logrus.Debug("local: opened destination handle: ", dstNsHandle, cfg.dstNetNsInode)

	/* Extract interface - source namespace */
	if err = setupLinkInNs(srcNsHandle, cfg.srcName, cfg.srcIP, nil, nil, false); err != nil {
		logrus.Errorf("local: failed to extract interface - source - %q: %v", cfg.srcName, err)
		return nil, err
	}

	/* Extract interface - destination namespace */
	if err = setupLinkInNs(dstNsHandle, cfg.dstName, cfg.dstIP, nil, nil, false); err != nil {
		logrus.Errorf("local: failed to extract interface - destination - %q: %v", cfg.dstName, err)
		return nil, err
	}

	/* Get a link object for the interface */
	ifaceLink, err := netlink.LinkByName(cfg.srcName)
	if err != nil {
		logrus.Errorf("local: failed to get link for %q - %v", cfg.srcName, err)
		return nil, err
	}

	/* Delete the VETH pair - host namespace */
	if err := netlink.LinkDel(ifaceLink); err != nil {
		logrus.Errorf("local: failed to delete the VETH pair - %v", err)
		return nil, err
	}

	logrus.Infof("local: deletion completed for devices - source: %s, destination: %s", cfg.srcName, cfg.dstName)
	srcDevice := monitoring.Device{Name: cfg.srcName, XconName: "SRC-" + cfg.id}
	dstDevice := monitoring.Device{Name: cfg.dstName, XconName: "DST-" + cfg.id}
	return map[string]monitoring.Device{cfg.srcNetNsInode: srcDevice, cfg.dstNetNsInode: dstDevice}, nil
}

// newVETH returns a VETH interface instance
func newVETH(srcName, dstName string) *netlink.Veth {
	/* Populate the VETH interface configuration */
	return &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: srcName,
			MTU:  cVETHMTU,
		},
		PeerName: dstName,
	}
}
