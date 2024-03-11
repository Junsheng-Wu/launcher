package utils

import (
	"fmt"
	"testing"

	"github.com/heimdalr/dag"
)

func TestOrderedWalk(t *testing.T) {
	d := dag.NewDAG()

	op1 := OperationSide{Id: "op1", Status: "Executable"}
	op2 := OperationSide{Id: "op2", Status: "Pending"}
	op3 := OperationSide{Id: "op3", Status: "Pending"}
	// 添加节点
	mapOps := map[string]*OperationSide{
		"op1": &op1,
		"op2": &op2,
		"op3": &op3,
	}

	sideMap := map[string][]string{
		"op1": {"op2"},

		"op2": {"op3"},
	}

	// 创建一个访问者
	visit := NewOperationVisitor(d, mapOps, sideMap)

	// 调用OrderedWalk方法
	d.OrderedWalk(visit)

	for name, ops := range visit.MapOps {
		fmt.Println(name, "-------", ops.Status)
	}
}

func TestOrderedWalkNoEdges(t *testing.T) {
	d := dag.NewDAG()

	// 添加节点
	op1 := OperationSide{Id: "op1", Status: "Executable"}
	op2 := OperationSide{Id: "op2", Status: "Pending"}
	op3 := OperationSide{Id: "op3", Status: "Pending"}
	d.AddVertex(op1)
	d.AddVertex(op2)
	d.AddVertex(op3)

	// 创建一个访问者

	mapOps := map[string]*OperationSide{
		"op1": &op1,
		"op2": &op2,
		"op3": &op3,
	}

	sideMap := map[string][]string{}
	visitor := NewOperationVisitor(d, mapOps, sideMap)

	// 调用OrderedWalk方法
	d.OrderedWalk(visitor)

	for k, v := range visitor.MapOps {
		fmt.Println(k, v)
	}

}

// other test case
func TestOrderedWalkComplexEdges(t *testing.T) {

}

// other test case
func TestOrderedWalkComplexEdges1(t *testing.T) {
	d := dag.NewDAG()

	// 添加节点
	op1 := OperationSide{Id: "op1", Status: "Succeeded"}
	op2 := OperationSide{Id: "op2", Status: "Pending"}
	op3 := OperationSide{Id: "op3", Status: "Pending"}
	op4 := OperationSide{Id: "op4", Status: "Pending"}
	op5 := OperationSide{Id: "op5", Status: "Pending"}
	op6 := OperationSide{Id: "op6", Status: "Pending"}
	op7 := OperationSide{Id: "op7", Status: "Pending"}
	op8 := OperationSide{Id: "op8", Status: "Pending"}
	op9 := OperationSide{Id: "op9", Status: "Succeeded"}
	op10 := OperationSide{Id: "op10", Status: "Succeeded"}
	op11 := OperationSide{Id: "op11", Status: "Pending"}

	//    1(success）        op9(s)         op10(p)
	//   /  \    \               \
	//  2(s) 3(p) 4(s)         op11(p)
	// /      \     \
	// 5(p)   6(p)   7(s)
	//                \
	//                 8(p）
	// 添加边

	mapOps := map[string]*OperationSide{
		"op1":  &op1,
		"op2":  &op2,
		"op3":  &op3,
		"op4":  &op4,
		"op5":  &op5,
		"op6":  &op6,
		"op7":  &op7,
		"op8":  &op8,
		"op9":  &op9,
		"op10": &op10,
		"op11": &op11,
	}

	sideMap := map[string][]string{
		"op1": {"op2", "op3", "op4"},
		"op2": {"op5"},
		"op3": {"op6"},
		"op4": {"op7"},
		"op7": {"op8"},
		"op9": {"op11"},
	}

	// 创建一个访问者
	visitor := NewOperationVisitor(d, mapOps, sideMap)

	// 调用OrderedWalk方法
	visitor.Dag.OrderedWalk(visitor)

	visitor.Dag.OrderedWalk(visitor)

	for k, v := range visitor.MapOps {
		fmt.Println(k, v)
	}


}

func TestOrderedWalkMoreComplexEdges(t *testing.T) {
	d := dag.NewDAG()

	// 添加节点
	op1 := OperationSide{Id: "op1", Status: "Executable"}
	op2 := OperationSide{Id: "op2", Status: "Executable"}
	op3 := OperationSide{Id: "op3", Status: "Executable"}
	op4 := OperationSide{Id: "op4", Status: "Executable"}
	op5 := OperationSide{Id: "op5", Status: "Executable"}
	op6 := OperationSide{Id: "op6", Status: "Executable"}
	op7 := OperationSide{Id: "op7", Status: "Executable"}
	// 	1
	// | \
	// 2  3
	// |  | \
	// 4  5  6
	//  \ | /
	//   7

	mapOps := map[string]*OperationSide{
		"op1": &op1,
		"op2": &op2,
		"op3": &op3,
		"op4": &op4,
		"op5": &op5,
		"op6": &op6,
		"op7": &op7,
	}

	sideMap := map[string][]string{
		"op1": {"op2", "op3"},
		"op2": {"op4"},
		"op3": {"op5"},
		"op4": {"op7"},
		"op7": {"op8"},
		"op9": {"op11"},
	}

	// 创建一个访问者
	visit := NewOperationVisitor(d, mapOps, sideMap)

	// 调用OrderedWalk方法
	d.OrderedWalk(visit)
}

// copy vistor_test.go dag case to there
func TestOrderedWalkComplexEdges2(t *testing.T) {
	// schematic diagram:
	//
	//	v5
	//	^
	//	|
	//	v4
	//	^
	//	|
	//	v2 --> v3
	//	^
	//	|
	//	v1
	d := dag.NewDAG()

	// 添加节点
	op1 := OperationSide{Id: "op1", Status: "Executable"}
	op2 := OperationSide{Id: "op1", Status: "Executable"}
	op3 := OperationSide{Id: "op1", Status: "Executable"}
	op4 := OperationSide{Id: "op1", Status: "Executable"}
	op5 := OperationSide{Id: "op1", Status: "Executable"}

	mapOps := map[string]*OperationSide{
		"op1": &op1,
		"op2": &op2,
		"op3": &op3,
		"op4": &op4,
		"op5": &op5,
	}

	sideMap := map[string][]string{
		"op1": {"op2"},
		"op2": {"op3", "op4"},
		"op4": {"op5"},
	}

	// 创建一个访问者
	visit := NewOperationVisitor(d, mapOps, sideMap)

	// 调用OrderedWalk方法
	d.OrderedWalk(visit)

}
