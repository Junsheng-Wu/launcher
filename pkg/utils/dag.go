package utils

import (
	ecnsv1 "easystack.com/plan/api/v1"
	"github.com/heimdalr/dag"
	clusteroperationv1alpha1 "github.com/kubean-io/kubean-api/apis/clusteroperation/v1alpha1"
)

type OperationSide struct {
	ClusterOps *ecnsv1.ClusterOps
	// ID is the unique identifier of the operation.
	Id string
	// Status is the current status of the operation.
	StatusChan chan string

	Status clusteroperationv1alpha1.OpsStatus
}

const (
	RunningStatus   clusteroperationv1alpha1.OpsStatus = "Running"
	SucceededStatus clusteroperationv1alpha1.OpsStatus = "Succeeded"
	FailedStatus    clusteroperationv1alpha1.OpsStatus = "Failed"
	Executable      clusteroperationv1alpha1.OpsStatus = "Executable"
)

func NewOperationSide(co ecnsv1.ClusterOps) *OperationSide {
	return &OperationSide{
		ClusterOps: &co,
		Id:         co.Name,
		Status:     co.Status,
	}
}

func (os OperationSide) ID() string {
	return os.Id
}

type OperationVisitor struct {
	Dag *dag.DAG
	// Value is the value of the operation of name.
	MapOps map[string]*OperationSide

	Sides map[string]string
}

func NewOperationVisitor(d *dag.DAG, operationSides map[string]*OperationSide, sideMap map[string]string) *OperationVisitor {
	for _, side := range operationSides {
		d.AddVertex(*side)
	}

	for i, j := range sideMap {
		d.AddEdge(i, j)
	}

	return &OperationVisitor{
		Dag:    d,
		MapOps: operationSides,
		Sides:  sideMap,
	}
}

func (o *OperationVisitor) Visit(v dag.Vertexer) {
	id, value := v.Vertex()
	if ok, _ := o.Dag.IsRoot(id); ok {
		if value.(OperationSide).Status == SucceededStatus || value.(OperationSide).Status == FailedStatus || value.(OperationSide).Status == RunningStatus {
			return
		} else {
			o.MapOps[id].Status = Executable
		}
	} else {
		parents, err := o.Dag.GetParents(id)
		if err != nil {
			return
		}
		for _, parent := range parents {
			if parent.(OperationSide).Status != SucceededStatus {
				return
			}
		}
		o.MapOps[id].Status = Executable
	}
}

func (o *OperationVisitor) GetExecutableSides() []OperationSide {
	var opsSides = []OperationSide{}
	for _, side := range o.MapOps {
		if side.Status == Executable {
			opsSides = append(opsSides, *side)
		}
	}
	return opsSides
}

func (o *OperationVisitor) GetFailedSides() []OperationSide {
	var opsSides = []OperationSide{}
	for _, side := range o.MapOps {
		if side.Status == FailedStatus {
			opsSides = append(opsSides, *side)
		}
	}
	return opsSides
}
