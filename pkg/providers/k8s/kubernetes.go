package k8s

import (
	"context"
	"fmt"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	errorutil "github.com/projectdiscovery/utils/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Provider is a data provider for gcp API
type Provider struct {
	id        string
	clientSet *kubernetes.Clientset
}

const (
	kubeconfig_file = "kubeconfig_file"
	kubeConfig      = "kubeconfig"
	providerName    = "kubernetes"
)

func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata("id")

	configFile, ok := options.GetMetadata(kubeconfig_file)
	configStr, strOk := options.GetMetadata(kubeConfig)

	if !ok && !strOk {
		return nil, errorutil.New("no kubeconfig_file or kubeconfig  provided")
	}
	context, _ := options.GetMetadata("context")

	var kubeConfig *rest.Config
	var err error
	if strOk {
		kubeConfig, err = buildConfigFromStr(context, configStr)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not build kubeconfig")
		}
	} else {
		kubeConfig, err = buildConfigWithContext(context, configFile)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not build kubeconfig")
		}
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create kubernetes clientset")
	}
	return &Provider{id: id, clientSet: clientset}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalList := schema.NewResources()
	services, err := p.clientSet.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not list kubernetes services")
	}
	k8sServiceProvider := K8sServiceProvider{serviceClient: services, id: p.id}
	serviceIPs, _ := k8sServiceProvider.GetResource(ctx)
	finalList.Merge(serviceIPs)

	ingress, err := p.clientSet.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not list kubernetes ingress")
	}
	k8sIngressProvider := K8sIngressProvider{ingress: ingress, id: p.id}
	ingressHosts, _ := k8sIngressProvider.GetResource(ctx)
	finalList.Merge(ingressHosts)
	return finalList, nil
}

func buildConfigWithContext(context string, kubeconfigPath string) (*rest.Config, error) {
	if context == "" {
		kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return kubeConfig, errorutil.NewWithErr(err).Msgf("could not read kubeconfig file")
		}
		return kubeConfig, nil
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

func buildConfigFromStr(contextName, configStr string) (*rest.Config, error) {

	clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(configStr))
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not read kubeconfig file")
	}
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not read kubeconfig file")
	}
	// Check if the context exists in the kubeconfig
	if _, exists := rawConfig.Contexts[contextName]; !exists {
		return nil, fmt.Errorf("context %q does not exist in the kubeconfig", contextName)
	}
	if contextName != "" {
		rawConfig.CurrentContext = contextName
	}
	// Create a new clientcmd.ClientConfig from the modified rawConfig
	modifiedClientConfig := clientcmd.NewNonInteractiveClientConfig(rawConfig, contextName, &clientcmd.ConfigOverrides{}, nil)

	// Get the rest.Config from the modified client configuration
	kubeConfig, err := modifiedClientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get client config: %v", err)
	}
	return kubeConfig, nil
}
