// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	apiv1 "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned/typed/api/v1"
	gentype "k8s.io/client-go/gentype"
)

// fakeSubscriptions implements SubscriptionInterface
type fakeSubscriptions struct {
	*gentype.FakeClientWithList[*v1.Subscription, *v1.SubscriptionList]
	Fake *FakePostgresqlV1
}

func newFakeSubscriptions(fake *FakePostgresqlV1, namespace string) apiv1.SubscriptionInterface {
	return &fakeSubscriptions{
		gentype.NewFakeClientWithList[*v1.Subscription, *v1.SubscriptionList](
			fake.Fake,
			namespace,
			v1.GroupVersion.WithResource("subscriptions"),
			v1.GroupVersion.WithKind("Subscription"),
			func() *v1.Subscription { return &v1.Subscription{} },
			func() *v1.SubscriptionList { return &v1.SubscriptionList{} },
			func(dst, src *v1.SubscriptionList) { dst.ListMeta = src.ListMeta },
			func(list *v1.SubscriptionList) []*v1.Subscription { return gentype.ToPointerSlice(list.Items) },
			func(list *v1.SubscriptionList, items []*v1.Subscription) {
				list.Items = gentype.FromPointerSlice(items)
			},
		),
		fake,
	}
}
