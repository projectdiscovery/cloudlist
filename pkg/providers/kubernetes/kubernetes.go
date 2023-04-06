package k8s

import (
	"context"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
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

const kubeconfig_file = "kubeconfig_file"
const providerName = "kubernetes"

func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata("id")

	// TODO : Add context filter to k8s provider
	configFile, ok := options.GetMetadata("kubeconfig_file")
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: kubeconfig_file}
	}
	context, _ := options.GetMetadata("context")
	kubeConfig, err := buildConfigWithContext(context, configFile)
	if err != nil {
		return nil, errors.Wrap(err, "could not build kubeconfig")
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, errors.Wrap(err, "could not create kubernetes clientset")
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
		return nil, errors.Wrap(err, "could not list kubernetes services")
	}
	k8sServiceProvider := k8sServiceProvider{serviceClient: services, id: p.id}
	serviceIPs, _ := k8sServiceProvider.GetResource(ctx)
	finalList.Merge(serviceIPs)

	ingress, err := p.clientSet.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "could not list kubernetes ingress")
	}
	k8sIngressProvider := k8sIngressProvider{ingress: ingress, id: p.id}
	ingressHosts, _ := k8sIngressProvider.GetResource(ctx)
	finalList.Merge(ingressHosts)
	return finalList, nil
}

func buildConfigWithContext(context string, kubeconfigPath string) (*rest.Config, error) {
	if context == "" {
		kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return kubeConfig, errors.Wrap(err, "could not read kubeconfig file")
		}
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}
