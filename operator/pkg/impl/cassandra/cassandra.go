package cassandra

import (
	"fmt"
	"reflect"

	"github.com/Netcracker/qubership-cassandra-operator/api/v1alpha1"
	"github.com/Netcracker/qubership-cassandra-operator/pkg/impl/utils"
	"github.com/Netcracker/qubership-nosqldb-operator-core/pkg/constants"
	"github.com/Netcracker/qubership-nosqldb-operator-core/pkg/core"
	"github.com/Netcracker/qubership-nosqldb-operator-core/pkg/steps"
	"go.uber.org/zap"
	v13 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Cassandra struct {
	core.MicroServiceCompound
}

func (r *Cassandra) Validate(ctx core.ExecutionContext) error {
	spec := ctx.Get(constants.ContextSpec).(*v1alpha1.CassandraDeployment)
	if reflect.ValueOf(spec).IsNil() {
		return &core.ExecutionError{Msg: "CassandraService CR spec is not found"}
	}
	return r.DefaultCompound.Validate(ctx)
}

type CassandraBuilder struct {
	core.ExecutableBuilder
}

func (r *CassandraBuilder) Build(ctx core.ExecutionContext) core.Executable {
	spec := ctx.Get(constants.ContextSpec).(*v1alpha1.CassandraDeployment)
	log := ctx.Get(constants.ContextLogger).(*zap.Logger)
	pvcSelector := map[string]string{
		utils.Service: utils.CassandraCluster,
	}

	cassandra := Cassandra{}
	cassandra.ServiceName = utils.Cassandra
	cassandra.CalcDeployType = func(ctx core.ExecutionContext) (core.MicroServiceDeployType, error) {
		request := ctx.Get(constants.ContextRequest).(reconcile.Request)

		helperImpl := ctx.Get(utils.KubernetesHelperImpl).(core.KubernetesHelper)

		pvcList := &v13.PersistentVolumeClaimList{}
		err := helperImpl.ListRuntimeObjectsByLabels(pvcList, request.Namespace, pvcSelector)
		var result core.MicroServiceDeployType
		if err != nil {
			result = core.Empty
		} else if len(pvcList.Items) == 0 {
			result = core.CleanDeploy
		} else {
			result = core.Update
		}

		log.Debug(fmt.Sprintf("%s deploy mode is used for %s service", result, utils.Cassandra))

		return result, err
	}

	//cassandra.AddStep(steps.NewMaintenanceConsulServiceStep(
	//	utils.Cassandra,
	//	CassandraConsulServiceRegistrationCast,
	//	true,
	//	"Deploying"))

	for index, dc := range utils.FilterDC(spec.Spec.Cassandra.DeploymentSchema.DataCenters, func(dc *v1alpha1.DataCenter) bool { return dc.Deploy }) {
		pvcContextFormat := fmt.Sprintf(utils.CassandraDCPvcNameFormat, index)
		nodesContext := fmt.Sprintf(utils.PVNodesFormat, index)
		replicas := dc.GetActiveReplicas()
		for storageIndex, storage := range dc.Storage {
			// PVC are stored in the context per storage
			pvcContext := fmt.Sprintf("%s-%v", pvcContextFormat, storageIndex)

			for _, replica := range replicas {
				pvcNameFormat := pvcContextFormat + "-%v"
				if storageIndex != 0 {
					// For any additional storages storage index in included
					pvcNameFormat = pvcNameFormat + fmt.Sprintf("-%v", storageIndex)
					// As the result the format for PVC is looks like that:
					// cassandra-data-dc%dcIndex%-%replicaIndex%-%storageIndexMoreThanZero%
				}
				pvcStep := &steps.CreatePVCStep{
					Storage:           storage,
					NameFormat:        pvcNameFormat,
					LabelSelector:     pvcSelector,
					ContextVarToStore: pvcContext,
					PVCCount: func(ctx core.ExecutionContext) int {
						return 1
					},
					WaitTimeout:  spec.Spec.WaitTimeout,
					Owner:        nil,
					WaitPVCBound: storage.WaitPVCBound,
					StartIndex:   replica,
				}

				if spec.Spec.DeletePVConUninstall {
					pvcStep.Owner = spec
				}

				cassandra.AddStep(pvcStep)
			}

			//Do it only for main storage. The rest should line with nodes from main storage
			if storageIndex == 0 {
				cassandra.AddStep(&steps.StoreNodesStep{
					Storage:           storage,
					ContextVarToStore: nodesContext,
				})
			}

			var tolerations []v13.Toleration
			if spec.Spec.Policies != nil {
				tolerations = spec.Spec.Policies.Tolerations
			}

			if spec.Spec.Recycler.Install {
				// Perform recycling for each pvc in each storage with the nodes from main storage
				cassandra.AddStep(&steps.PVRecyclerStep{
					DockerImage:        spec.Spec.Cassandra.DockerImage,
					Volumes:            storage.Volumes,
					Tolerations:        tolerations,
					PVCContextVar:      pvcContext,
					PVNodesContextVar:  nodesContext,
					WaitTimeout:        spec.Spec.WaitTimeout,
					PodSecurityContext: spec.Spec.PodSecurityContext,
					Resources:          spec.Spec.Recycler.Resources,
					Owner:              spec,
				})
			}
		}

	}

	cassandra.AddStep(&CassandraServicesStep{})
	cassandra.AddStep(&CassandraLoadbalancerService{})

	cassandra.AddStep(&CassandraStatefulSetStep{})

	cassandra.AddStep(&CreateSuperUser{
		Username: spec.Spec.User,
		Password: func() string { return ctx.Get(utils.ContextPasswordKey).(string) },
	})

	cassandra.AddStep(&UpdateCassandraCredentials{})

	cassandra.AddStep(&RemoveNodes{})

	cassandra.AddStep(&CleanupNodes{})

	cassandra.AddStep(&UpdateSystemKeyspacesTopology{})

	cassandra.AddStep(&CassandraReaper{})

	cassandra.AddStep(&NodetoolRebuild{})

	cassandra.AddStep(&DropCassandraDefaultUser{})

	//cassandra.AddStep(steps.NewMaintenanceConsulServiceStep(
	//	utils.Cassandra,
	//	CassandraConsulServiceRegistrationCast,
	//	false))

	return &cassandra
}

func (r *Cassandra) Condition(ctx core.ExecutionContext) (bool, error) {
	spec := ctx.Get(constants.ContextSpec).(*v1alpha1.CassandraDeployment)
	microServiceCheck, microserviceCheckErr := core.CheckSpecChange(ctx, spec.Spec.Cassandra, utils.Cassandra)
	commonCheck := ctx.Get(constants.IsAnyCommonParameterChanged).(bool)

	if microserviceCheckErr != nil {
		return microServiceCheck, microserviceCheckErr
	} else {
		return microServiceCheck || commonCheck, nil
	}
}
