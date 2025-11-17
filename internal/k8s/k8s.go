package k8s

import (
	"context"
	"time"

	"bud.studio/stove8s/internal/oci"
	"github.com/google/go-containerregistry/pkg/authn"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ImagePushSecretGet(k8sClient *kubernetes.Clientset, namespace, secretName, registry string) (authn.Authenticator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	secret, err := k8sClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return oci.AuthFromK8sSecret(secret, registry)
}
