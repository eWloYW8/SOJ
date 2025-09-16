package types

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/logrusorgru/aurora/v4"
)

// Config 全局配置
type Config struct {
	HostKey    string `yaml:"HostKey"`
	ListenAddr string `yaml:"ListenAddr"`
	APIAddr    string `yaml:"APIAddr"`

	AllowedSSHPubkey string `yaml:"AllowedSSHPubkey"`

	SubmitsDir    string `yaml:"SubmitsDir"`
	SubmitWorkDir string `yaml:"SubmitWorkDir"`
	ProblemsDir   string `yaml:"ProblemsDir"`

	RealSubmitsDir    string `yaml:"RealSubmitsDir"`
	RealSubmitWorkDir string `yaml:"RealSubmitWorkDir"`

	SqlitePath string `yaml:"SqlitePath"`

	DockerCli        string `yaml:"DockerCli"`
	ProblemURLPrefix string `yaml:"ProblemURLPrefix"`

	SubmitGid int `yaml:"SubmitGid"`
	SubmitUid int `yaml:"SubmitUid"`

	Admins []string `yaml:"Admins"`
}

// JudgeResult 评测结果
type JudgeResult struct {
	Success bool    `json:"success"`
	Score   float64 `json:"score"`
	Msg     string  `json:"message"`
	Memory  uint64  `json:"memory"` // in bytes
	Time    uint64  `json:"time"`   // in ns
}

// WorkflowResult 工作流结果
type WorkflowResult struct {
	Success  bool                 `json:"success"`
	Logs     string               `json:"logs"`
	ExitCode int                  `json:"exit_code"`
	Steps    []WorkflowStepResult `json:"steps"`
}

// WorkflowStepResult 工作流步骤结果
type WorkflowStepResult struct {
	Logs     string `json:"logs"`
	ExitCode int    `json:"exit_code"`
}

// Userface 用户界面包装器
type Userface struct {
	*bytes.Buffer
	io.Writer
}

func (f Userface) Println(a ...interface{}) (n int, err error) {
	return fmt.Fprintln(f, a...)
}

func (f Userface) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(f, format, a...)
}

func (f Userface) Write(p []byte) (n int, err error) {
	var _f io.Writer
	if f.Writer != nil {
		_f = io.MultiWriter(f.Buffer, f.Writer)
	} else {
		_f = f.Buffer
	}
	_f.Write(p)
	return len(p), nil
}

// SubmitHash 提交文件哈希
type SubmitHash struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// SubmitCtx 提交上下文
type SubmitCtx struct {
	ID      string `gorm:"primaryKey" json:"id"`
	User    string `json:"user"`
	Problem string `json:"problem"`

	SubmitTime int64 `json:"submit_time"`
	LastUpdate int64 `json:"last_update"`

	Status string `json:"status"`
	Msg    string `json:"message"`

	SubmitDir       string          `json:"-"`
	SubmitsHashes   SubmitsHashes   `json:"submits_hashes"`
	Workdir         string          `json:"-"`
	WorkflowResults WorkflowResults `json:"workflow_results"`
	JudgeResult     JudgeResult     `json:"judge_result"`

	RealWorkdir string `json:"-"`

	Running  chan struct{} `gorm:"-" json:"-"`
	Userface Userface      `json:"-"`
}

func (ctx *SubmitCtx) SetStatus(status string) *SubmitCtx {
	ctx.Status = status
	ctx.LastUpdate = time.Now().UnixNano()
	return ctx
}

func (ctx *SubmitCtx) SetMsg(msg string) *SubmitCtx {
	ctx.Msg = msg
	ctx.LastUpdate = time.Now().UnixNano()
	return ctx
}

// Problem 问题定义
type Problem struct {
	Version  int        `yaml:"version"`
	Id       string     `yaml:"id"`
	Text     string     `yaml:"text"`
	Weight   float64    `yaml:"weight"`
	Submits  []Submit   `yaml:"submits"`
	Workflow []Workflow `yaml:"workflow"`
}

// Submit 提交定义
type Submit struct {
	Path  string `yaml:"path"`
	IsDir bool   `yaml:"isdir"`
}

// Workflow 工作流定义
type Workflow struct {
	Image           string   `yaml:"image"`
	Steps           []string `yaml:"steps"`
	Timeout         int      `yaml:"timeout"`
	Root            bool     `yaml:"root"`
	DisableNetwork  bool     `yaml:"disablenetwork"`
	Show            []int    `yaml:"show"`
	PrivilegedSteps []int    `yaml:"privilegedsteps"`
	NetworkHostMode bool     `yaml:"networkhostmode"`
	Mounts          []Mount  `yaml:"mounts"`
}

// Mount 挂载定义
type Mount struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"readonly"`
}

// User 用户信息
type User struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	Token          string         `gorm:"uniqueIndex" json:"-"`
	BestScores     JMapStrFloat64 `json:"best_scores"`
	BestSubmits    JMapStrString  `json:"best_submits"`
	BestSubmitDate JMapStrInt64   `json:"best_submit_date"`
	TotalScore     float64        `json:"total_score"`
}

func (u *User) CalculateTotalScore() {
	var total float64
	for _, s := range u.BestScores {
		total += s
	}
	u.TotalScore = total
}

// 辅助函数
func GetTime(t time.Time) aurora.Value {
	return aurora.Gray(15, t.Format("2006-01-02 15:04:05.000"))
}

func ColorizeScore(res JudgeResult) aurora.Value {
	if !res.Success {
		return aurora.Gray(15, res.Score)
	}
	if res.Score >= 95 {
		return aurora.Green(res.Score)
	} else if res.Score >= 60 {
		return aurora.Yellow(res.Score)
	} else {
		return aurora.Red(res.Score)
	}
}

func ColorizeStatus(status string) aurora.Value {
	switch status {
	case "init":
		return aurora.Gray(10, status)
	case "pending":
		return aurora.Cyan(status)
	case "prep_dirs":
		return aurora.Yellow(status)
	case "prep_files":
		return aurora.Yellow(status)
	case "run_workflow":
		return aurora.Yellow(status)
	case "collect_result":
		return aurora.Yellow(status)
	case "completed":
		return aurora.Green(status)
	case "failed":
		return aurora.Red(status)
	case "dead":
		return aurora.Gray(15, status)
	default:
		return aurora.Bold(status)
	}
}

// 数据库类型定义
type JMapStrFloat64 map[string]float64
type JMapStrString map[string]string
type JMapStrInt64 map[string]int64
type SubmitsHashes []SubmitHash
type WorkflowResults []WorkflowResult

// 数据库序列化接口实现
func (sh SubmitHash) Value() (driver.Value, error) {
	return json.Marshal(sh)
}

func (sh *SubmitHash) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, sh)
	}
	return json.Unmarshal(b, sh)
}

func (sh SubmitsHashes) Value() (driver.Value, error) {
	return json.Marshal(sh)
}

func (sh *SubmitsHashes) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, sh)
	}
	return json.Unmarshal(b, sh)
}

func (sh WorkflowResult) Value() (driver.Value, error) {
	return json.Marshal(sh)
}

func (sh *WorkflowResult) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, sh)
	}
	return json.Unmarshal(b, sh)
}

func (sh WorkflowResults) Value() (driver.Value, error) {
	return json.Marshal(sh)
}

func (sh *WorkflowResults) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, sh)
	}
	return json.Unmarshal(b, sh)
}

func (sh JudgeResult) Value() (driver.Value, error) {
	return json.Marshal(sh)
}

func (sh *JudgeResult) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, sh)
	}
	return json.Unmarshal(b, sh)
}

func (sh Userface) Value() (driver.Value, error) {
	return sh.Buffer.String(), nil
}

func (sh *Userface) Scan(value interface{}) error {
	b, ok := value.(string)
	if !ok {
		return json.Unmarshal([]byte(b), sh)
	}
	sh.Buffer = bytes.NewBufferString(b)
	return nil
}

func (u JMapStrFloat64) Value() (driver.Value, error) {
	return json.Marshal(u)
}

func (u *JMapStrFloat64) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, u)
	}
	return json.Unmarshal(b, u)
}

func (u JMapStrString) Value() (driver.Value, error) {
	return json.Marshal(u)
}

func (u *JMapStrString) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, u)
	}
	return json.Unmarshal(b, u)
}

func (u JMapStrInt64) Value() (driver.Value, error) {
	return json.Marshal(u)
}

func (u *JMapStrInt64) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return json.Unmarshal(b, u)
	}
	return json.Unmarshal(b, u)
}
