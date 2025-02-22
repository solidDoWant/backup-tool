// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	apiv1 "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned/typed/api/v1"
	gentype "k8s.io/client-go/gentype"
)

// fakeBackups implements BackupInterface
type fakeBackups struct {
	*gentype.FakeClientWithList[*v1.Backup, *v1.BackupList]
	Fake *FakePostgresqlV1
}

func newFakeBackups(fake *FakePostgresqlV1, namespace string) apiv1.BackupInterface {
	return &fakeBackups{
		gentype.NewFakeClientWithList[*v1.Backup, *v1.BackupList](
			fake.Fake,
			namespace,
			v1.GroupVersion.WithResource("backups"),
			v1.GroupVersion.WithKind("Backup"),
			func() *v1.Backup { return &v1.Backup{} },
			func() *v1.BackupList { return &v1.BackupList{} },
			func(dst, src *v1.BackupList) { dst.ListMeta = src.ListMeta },
			func(list *v1.BackupList) []*v1.Backup { return gentype.ToPointerSlice(list.Items) },
			func(list *v1.BackupList, items []*v1.Backup) { list.Items = gentype.FromPointerSlice(items) },
		),
		fake,
	}
}
