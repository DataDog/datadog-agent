package k8s

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps a Kubernetes clientset with additional functionality
type Client struct {
	clientset *kubernetes.Clientset
	context   string
}

// NewClient creates a new Kubernetes client with the specified kubeconfig and context
func NewClient(kubeconfigPath, context string) (*Client, error) {
	// Load kubeconfig
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	client := &Client{
		clientset: clientset,
		context:   context,
	}

	return client, nil
}

// HealthCheck verifies connectivity to the Kubernetes cluster
func (c *Client) HealthCheck(ctx context.Context) error {
	// Try to get server version as a health check
	_, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to kubernetes cluster: %w", err)
	}
	return nil
}

// Clientset returns the underlying Kubernetes clientset
func (c *Client) Clientset() *kubernetes.Clientset {
	return c.clientset
}

// Context returns the current Kubernetes context
func (c *Client) Context() string {
	return c.context
}
