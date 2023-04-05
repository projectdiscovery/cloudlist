package k8s

import (
	"context"
	"fmt"
	"os"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Provider is a data provider for gcp API
type Provider struct {
	id        string
	clientSet *kubernetes.Clientset
}

const kubeconfig_file = "kubeconfig_file"
const providerName = "k8s"

func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata("id")

	// TODO : Add context filter to k8s provider
	configFile, _ := options.GetMetadata("kubeconfig_file")
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", configFile)
	if err != nil {
		fmt.Printf("error getting Kubernetes config: %v\n", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		panic(err.Error())
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
	// fmt.Println("Inside resources")
	services, err := p.clientSet.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	k8sServiceProvider := k8sServiceProvider{serviceClient: services}
	ret, err := k8sServiceProvider.GetResource(ctx)
	return ret, err
}
