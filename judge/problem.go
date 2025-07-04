package judge

import (
	"log"
	"os"

	"github.com/mrhaoxx/SOJ/types"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// ProblemManager 问题管理器
type ProblemManager struct {
	problems map[string]types.Problem
	pblms    []string
}

// NewProblemManager 创建新的问题管理器
func NewProblemManager() *ProblemManager {
	return &ProblemManager{
		problems: make(map[string]types.Problem),
		pblms:    make([]string, 0),
	}
}

// LoadProblem 加载单个问题
func (pm *ProblemManager) LoadProblem(file string) types.Problem {
	_f, err := os.ReadFile(file)

	if err != nil {
		panic(err)
	}

	var _p types.Problem

	err = yaml.Unmarshal(_f, &_p)

	if err != nil {
		panic(errors.Wrap(err, "failed to unmarshal problem "+file))
	}

	if _p.Weight == 0 {
		_p.Weight = 1.0
	}

	pm.pblms = append(pm.pblms, _p.Id)
	pm.problems[_p.Id] = _p
	return _p
}

// LoadProblemDir 从目录加载所有问题
func (pm *ProblemManager) LoadProblemDir(dir string) map[string]types.Problem {
	_f, err := os.ReadDir(dir)

	if err != nil {
		panic(err)
	}

	pm.problems = make(map[string]types.Problem)
	pm.pblms = make([]string, 0)

	for _, f := range _f {
		var _pf = pm.LoadProblem(dir + "/" + f.Name())
		pm.problems[_pf.Id] = _pf
		log.Println("loaded problem", _pf.Id)
	}

	return pm.problems
}

// GetProblem 获取问题
func (pm *ProblemManager) GetProblem(id string) (types.Problem, bool) {
	p, ok := pm.problems[id]
	return p, ok
}

// GetAllProblems 获取所有问题
func (pm *ProblemManager) GetAllProblems() map[string]types.Problem {
	return pm.problems
}

// GetProblemList 获取问题列表
func (pm *ProblemManager) GetProblemList() []string {
	return pm.pblms
}
