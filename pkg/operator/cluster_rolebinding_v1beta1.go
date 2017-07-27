package operator

import (
	"errors"
	"reflect"

	acrt "github.com/appscode/go/runtime"
	"github.com/appscode/kubed/pkg/util"
	"github.com/appscode/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	rbac "k8s.io/client-go/pkg/apis/rbac/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// Blocks caller. Intended to be called as a Go routine.
func (op *Operator) WatchClusterRoleBindingV1beta1() {
	if !util.IsPreferredAPIResource(op.KubeClient, apiv1.SchemeGroupVersion.String(), "ClusterRoleBinding") {
		log.Warningf("Skipping watching non-preferred GroupVersion:%s Kind:%s", apiv1.SchemeGroupVersion.String(), "ClusterRoleBinding")
		return
	}

	defer acrt.HandleCrash()

	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return op.KubeClient.RbacV1beta1().ClusterRoleBindings().List(metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return op.KubeClient.RbacV1beta1().ClusterRoleBindings().Watch(metav1.ListOptions{})
		},
	}
	_, ctrl := cache.NewInformer(lw,
		&rbac.ClusterRoleBinding{},
		op.syncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if res, ok := obj.(*rbac.ClusterRoleBinding); ok {
					log.Infof("ClusterRoleBinding %s@%s added", res.Name, res.Namespace)
					util.AssignTypeKind(res)

					if op.Opt.EnableSearchIndex {
						if err := op.SearchIndex.HandleAdd(obj); err != nil {
							log.Errorln(err)
						}
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				if res, ok := obj.(*rbac.ClusterRoleBinding); ok {
					log.Infof("ClusterRoleBinding %s@%s deleted", res.Name, res.Namespace)
					util.AssignTypeKind(res)

					if op.Opt.EnableSearchIndex {
						if err := op.SearchIndex.HandleDelete(obj); err != nil {
							log.Errorln(err)
						}
					}
					if op.TrashCan != nil {
						op.TrashCan.Delete(res.TypeMeta, res.ObjectMeta, obj)
					}
				}
			},
			UpdateFunc: func(old, new interface{}) {
				oldRes, ok := old.(*rbac.ClusterRoleBinding)
				if !ok {
					log.Errorln(errors.New("Invalid ClusterRoleBinding object"))
					return
				}
				newRes, ok := new.(*rbac.ClusterRoleBinding)
				if !ok {
					log.Errorln(errors.New("Invalid ClusterRoleBinding object"))
					return
				}
				util.AssignTypeKind(oldRes)
				util.AssignTypeKind(newRes)

				if op.Opt.EnableSearchIndex {
					op.SearchIndex.HandleUpdate(old, new)
				}
				if op.TrashCan != nil && op.Config.RecycleBin.HandleUpdates {
					if !reflect.DeepEqual(oldRes.Labels, newRes.Labels) ||
						!reflect.DeepEqual(oldRes.Annotations, newRes.Annotations) ||
						!reflect.DeepEqual(oldRes.RoleRef, newRes.RoleRef) ||
						!reflect.DeepEqual(oldRes.Subjects, newRes.Subjects) {
						op.TrashCan.Update(newRes.TypeMeta, newRes.ObjectMeta, old, new)
					}
				}
			},
		},
	)
	ctrl.Run(wait.NeverStop)
}