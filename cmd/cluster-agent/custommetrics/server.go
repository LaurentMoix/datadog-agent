// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	//"fmt"
	//"os"
	//"time"

	//"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd/server"

	//apimeta "k8s.io/apimachinery/pkg/api/meta"
	//"k8s.io/client-go/discovery"
	//"k8s.io/client-go/dynamic"
	//"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
	"github.com/prometheus/common/log"
	//options "k8s.io/apiserver/pkg/server/options"
)

//var options *server.CustomMetricsAdapterServerOptions
var stopCh chan struct{}

//func init() {
//	// FIXME: log to seelog
//	options = server.NewCustomMetricsAdapterServerOptions(os.Stdout, os.Stdout)
//}

type DatadogMetricsAdapter struct {
	basecmd.AdapterBase

	// the message printed on startup
	Message string
}

// AddFlags ensures the required flags exist
//func AddFlags(fs *pflag.FlagSet) {
//	options.SecureServing.AddFlags(fs)
//	options.Authentication.AddFlags(fs)
//	options.Authorization.AddFlags(fs)
//	options.Features.AddFlags(fs)
//}

//// ValidateArgs validates the custom metrics arguments passed
//func ValidateArgs(args []string) error {
//	return options.Validate(args)
//}

// StartServer creates and start a k8s custom metrics API server
func StartServer() error {

	cmd := &DatadogMetricsAdapter{}
	log.Infof("adapter init %#v", cmd)
	cmd.InstallFlags()
	log.Infof("adapter installed flags %#v", cmd)

	//NewDelegatingAuthenticationOptions()
	//auth := options.NewDelegatingAuthenticationOptions()
	//auth.RequestHeader =
	a, e := cmd.Authentication.ToAuthenticationConfig()

	if e != nil {
		log.Infof("err while authenticating %#v", e)
	}
	log.Infof("Authenticated with %#v", a)
	log.Infof("")
	err := cmd.Authorization.Validate()
	for _, e := range err {
		log.Debugf("err: %v", e)
	}

	//authenticator.
	provider := cmd.makeProviderOrDie()
	cmd.WithExternalMetrics(provider)

	if err := cmd.Run(wait.NeverStop); err != nil {
		log.Errorf("unable to run custom metrics adapter: %v", err)
		return err
	}
    return nil
	//config, err := options.Config()
	//if err != nil {
	//	return err
	//}
	//var clientConfig *rest.Config
	//clientConfig, err = rest.InClusterConfig()
	//if err != nil {
	//	return err
	//}
	//
	//discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	//if err != nil {
	//	return fmt.Errorf("unable to construct discovery client for dynamic client: %v", err)
	//}

	//dynamicMapper, err := dynamicmapper.NewRESTMapper(discoveryClient, apimeta.InterfacesForUnstructured, time.Second*5)
	//if err != nil {
	//	return fmt.Errorf("unable to construct dynamic discovery mapper: %v", err)
	//}
	//
	//clientPool := dynamic.NewClientPool(clientConfig, dynamicMapper, dynamic.LegacyAPIPathResolverFunc)
	//if err != nil {
	//	return fmt.Errorf("unable to construct lister client to initialize provider: %v", err)
	//}
	//
	//client, err := as.GetAPIClient()
	//if err != nil {
	//	return err
	//}
	//datadogHPAConfigMap := custommetrics.GetConfigmapName()
	//store, err := custommetrics.NewConfigMapStore(client.Cl, common.GetResourcesNamespace(), datadogHPAConfigMap)
	//if err != nil {
	//	return err
	//}
	//emProvider := custommetrics.NewDatadogProvider(clientPool, dynamicMapper, store)
	//// As the Custom Metrics Provider is introduced, change the first emProvider to a cmProvider.
	//server, err := config.Complete().New("datadog-custom-metrics-adapter", emProvider, emProvider)
	//if err != nil {
	//	return err
	//}
	//stopCh = make(chan struct{})
	//return server.GenericAPIServer.PrepareRun().Run(stopCh)
}

func (a *DatadogMetricsAdapter) makeProviderOrDie() provider.ExternalMetricsProvider {
	client, err := a.DynamicClient()


	if err != nil {
		glog.Fatalf("unable to construct dynamic client: %v", err)
	}
	apiCl, err := as.GetAPIClient()
	if err != nil {
		return nil
	}
	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	store, err := custommetrics.NewConfigMapStore(apiCl.Cl, common.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		return nil
	}

	mapper, err := a.RESTMapper()
	if err != nil {
		glog.Fatalf("unable to construct discovery REST mapper: %v", err)
	}

	return custommetrics.NewDatadogProvider(client, mapper, store)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if stopCh != nil {
		close(stopCh)
	}
}
