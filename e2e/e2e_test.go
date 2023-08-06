package e2e_test

import (
	"context"
	"fmt"
	"testing"

	kubeshufflesharderiov1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"github.com/soggycactus/kube-shuffle-sharder/shuffleshard"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	numTenants        = 6
	numShardsPossible = 6
	shardSize         = 2
	tenantLabel       = "kube-shuffle-sharder.io/tenant"
	namespaceLabel    = "kube-shuffle-sharder.io/affinity-injection"
	podImage          = "nginx:1.14.2"
	namespace         = "e2e-test"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kubeshufflesharderiov1.AddToScheme(scheme))
}

func TestE2E(t *testing.T) {
	ctx := context.Background()
	config := controllerruntime.GetConfigOrDie()
	mgr, err := controllerruntime.NewManager(config, manager.Options{
		Scheme: scheme,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	go mgr.Start(ctx)

	clientset := mgr.GetClient()

	if err := clientset.Create(
		ctx,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Labels: map[string]string{
					namespaceLabel: "enabled",
				},
			},
		}); client.IgnoreAlreadyExists(err) != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	var tenants []string
	for i := 0; i < numTenants; i++ {
		tenants = append(tenants, fmt.Sprintf("tenant-%d", i))
	}

	var pods []corev1.Pod
	for _, tenant := range tenants {
		pods = append(pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tenant,
				Namespace: namespace,
				Labels: map[string]string{
					tenantLabel: tenant,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  tenant,
						Image: podImage,
					},
				},
			},
		})
	}

	// Assert pods can be created
	for _, pod := range pods {
		assert.NoError(t, clientset.Create(ctx, &pod), "pod creation should succeed")
	}

	for _, tenant := range tenants {
		shard := kubeshufflesharderiov1.ShuffleShard{
			ObjectMeta: metav1.ObjectMeta{
				Name: tenant,
			},
		}
		// Assert that all shuffle shards exist
		assert.NoError(t, clientset.Get(ctx, types.NamespacedName{Name: tenant}, &shard))

		// Assert that the spec is accurate
		assert.Len(t, shard.Spec.NodeGroups, shardSize, "shard size should match")
		assert.Equal(t, tenant, shard.Spec.Tenant, "tenant should match")

		shard.Spec.NodeGroups = []string{"new", "value"}
		assert.ErrorContains(t, clientset.Update(ctx, &shard), kubeshufflesharderiov1.ErrShuffleShardIsImmutable.Error(), "ShuffleShard should be immutable")
	}

	// create an additional pod for a tenant & assert that they have the same shuffle shard
	extraPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-2", tenants[0]),
			Namespace: namespace,
			Labels: map[string]string{
				tenantLabel: tenants[0],
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  tenants[0],
					Image: podImage,
				},
			},
		},
	}
	assert.NoError(t, clientset.Create(ctx, &extraPod), "pod creation should succeed")
	assert.NoError(t, clientset.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-2", tenants[0]), Namespace: namespace}, &extraPod), "should be able to get pod")
	var originalPod corev1.Pod
	assert.NoError(t, clientset.Get(ctx, types.NamespacedName{Name: tenants[0], Namespace: namespace}, &originalPod), "should be able to get pod")
	assert.Equal(
		t,
		originalPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
		extraPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
		"selector terms should match",
	)

	// try to create another tenant & assert that no shards are left
	newTenant := fmt.Sprintf("tenant-%d", numTenants)
	newTenantPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newTenant,
			Namespace: namespace,
			Labels: map[string]string{
				tenantLabel: newTenant,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  newTenant,
					Image: podImage,
				},
			},
		},
	}
	assert.ErrorContains(t, clientset.Create(ctx, &newTenantPod), shuffleshard.ErrNoShardsAvailable.Error(), "should not be able create another ShuffleShard")

	// clean up pods
	var podList corev1.PodList
	assert.NoError(t, clientset.List(ctx, &podList, &client.ListOptions{Namespace: namespace}))
	for _, pod := range podList.Items {
		assert.NoError(t, clientset.Delete(ctx, &pod))
	}

	// clean up shuffleshards
	var shardList kubeshufflesharderiov1.ShuffleShardList
	assert.NoError(t, clientset.List(ctx, &shardList))
	assert.Len(t, shardList.Items, numShardsPossible, "should match total possible shards")
	for _, shard := range shardList.Items {
		assert.NoError(t, clientset.Delete(ctx, &shard))
	}
}
