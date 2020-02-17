from vnc_api.vnc_api import VncApi, VirtualMachine, VirtualMachineInterface, PermType2, InstanceIp
from vnc_api.exceptions import RefsExistError
from cfgm_common import PERMS_RWX
import uuid
import sys
import json

api = VncApi('admin', 'contrail123', 'defaulttenant', ['10.0.2.15'], 8082, '/')
vnet = sys.argv[2]
vn_fq_name = vnet.split('.')
vm_name = sys.argv[1]
vm_display_name = vm_name + "displayname"
vmi_name = sys.argv[1] + "_vmi"
vmi_display_name = vmi_name + "displayname"
vr_uuid = 'b2ae41dc-1386-4e05-bf88-4ccace306bbf'
iip_name = vmi_name + "iip"

## GET PROJECT
proj_fq_name = ["default-domain","k8s-default"]
proj_obj = api.project_read(fq_name=proj_fq_name)
print >> sys.stderr, "Fetched project: ", proj_obj

## GET VN
vn_obj = None
vn_obj = api.virtual_network_read(fq_name=vn_fq_name)
ipam = vn_obj.get_network_ipam_refs()
vn_uuid = vn_obj.uuid
subnet_uuid = ipam[0]["attr"].ipam_subnets[0].subnet_uuid
print >> sys.stderr, "Fetched VN: ", vn_obj

## CREATE VM
proj_uuid = proj_obj.uuid
vm_obj = None
vm_uuid = None
pod_uuid = str(uuid.uuid1())
perms2 = PermType2()
perms2.owner = proj_uuid
perms2.owner_access = PERMS_RWX
vm_obj = VirtualMachine(name=vm_name, perms2=perms2, display_name=vm_display_name, parent_obj=proj_obj)
vm_obj.uuid = pod_uuid
vm_obj.set_server_type("container")
try:
    vm_response = api.virtual_machine_create(vm_obj)
    print >> sys.stderr, "Created VM: ", vm_response
    vm_uuid = vm_response
except RefsExistError as ref:
    print >> sys.stderr, "Not creating VM, already exists: ", str(ref)
    vm_obj = api.virtual_machine_read(fq_name=[vm_name])
    print >> sys.stderr, vm_obj
    vm_uuid = vm_obj.uuid

## CREATE VMI
obj_uuid = str(uuid.uuid1())
vmi_prop = None

vmi_obj = VirtualMachineInterface(
            name=vmi_name, parent_obj=proj_obj,
            virtual_machine_interface_properties=vmi_prop,
            display_name=vmi_display_name)

vmi_obj.uuid = obj_uuid
vmi_obj.set_virtual_network(vn_obj)
vmi_obj.set_virtual_machine(vm_obj)
try:
    vmi_response = api.virtual_machine_interface_create(vmi_obj)
    print >> sys.stderr, "Created VMI:", vmi_response
    vmi_uuid = vmi_response
except RefsExistError as ref:
    print >> sys.stderr, "Not creating VMI, already exists: ", str(ref)
    vmi_obj = api.virtual_machine_interface_read(fq_name=proj_fq_name + [vmi_name])
    vmi_uuid = vmi_obj.uuid

## LINK VM TO VROUTER
vrouter_obj = api.virtual_router_read(id=vr_uuid)
ref_response = api.ref_update('virtual-router', vrouter_obj.uuid,
            'virtual-machine', vm_obj.uuid, None, 'ADD')
print >> sys.stderr, "Linked VM to vRouter:", ref_response

## CREATE INTERFACE IP

iip_uuid = str(uuid.uuid1())
perms2 = PermType2()
perms2.owner = proj_uuid
perms2.owner_access = PERMS_RWX
iip_obj = InstanceIp(name=iip_name, subnet_uuid=subnet_uuid,
                     display_name=iip_name, perms2=perms2)
iip_obj.uuid = iip_uuid
iip_obj.add_virtual_network(vn_obj)

iip_obj.add_virtual_machine_interface(vmi_obj)

try:
    api.instance_ip_create(iip_obj)
except RefsExistError as ref:
    print >> sys.stderr, "Not creating VMI IP, already exists: ", str(ref)

print json.dumps({"vmiUuid": vmi_uuid, "vmUuid": vm_uuid, "vnUuid": vn_uuid})
