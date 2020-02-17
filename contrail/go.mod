module github.com/networkservicemesh/networkservicemesh/contrail

go 1.13

replace (

github.com/census-instrumentation/opencensus-proto v0.1.0-0.20181214143942-ba49f56771b8 => github.com/census-instrumentation/opencensus-proto v0.0.3-0.20181214143942-ba49f56771b8
github.com/containernetworking/cni v0.8.0 => github.com/containernetworking/cni v0.5.3-0.20170603124728-98826b72cc3a

)

require github.com/containernetworking/cni v0.5.3-0.20170603124728-98826b72cc3a // indirect
