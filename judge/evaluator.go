package judge

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mrhaoxx/SOJ/types"
	"github.com/pkg/errors"

	"github.com/logrusorgru/aurora/v4"
	"github.com/rs/zerolog/log"

	"github.com/docker/docker/api/types/mount"
)

// Evaluator 评测器
type Evaluator struct {
	cfg       *types.Config
	docker    DockerInterface
	dbService *types.DatabaseService
}

// DockerInterface Docker接口
type DockerInterface interface {
	RunImage(name string, user string, hostname string, image string, workdir string, mounts []mount.Mount, mask bool, ReadonlyRootfs bool, networkdisabled bool, timeout int, networkhosted bool, env []string) (ok bool, id string)
	CleanContainer(id string)
	ExecContainer(id string, cmd string, timeout int, stdout, stderr io.Writer, env []string, privileged bool) (int, string, error)
	GetContainerLogs(id string) (string, error)
}

// NewEvaluator 创建新的评测器
func NewEvaluator(cfg *types.Config, docker DockerInterface, dbService *types.DatabaseService) *Evaluator {
	return &Evaluator{
		cfg:       cfg,
		docker:    docker,
		dbService: dbService,
	}
}

// RunJudge 运行评测
func (e *Evaluator) RunJudge(ctx *types.SubmitCtx, problem *types.Problem) {
	log.Debug().Timestamp().Str("id", ctx.ID).Str("user", ctx.User).Str("problem", ctx.Problem).Msg("run judge")

	// var start_time = time.Now()
	var err error

	defer func() {
		log.Debug().Timestamp().Str("id", ctx.ID).Str("status", ctx.Status).Str("judgemsg", ctx.Msg).AnErr("err", err).Msg("judge finished")
		ctx.Userface.Println(types.GetTime(time.Now()), "Submission", types.ColorizeStatus(ctx.Status))
		close(ctx.Running)
		e.dbService.UpdateSubmit(ctx)
	}()

	ctx.Userface.Println("Submission ID:", aurora.Magenta(ctx.ID))

	// 首先设置为pending状态，等待资源准备
	ctx.SetStatus("pending").SetMsg("submission is pending, waiting for judge resources")
	e.dbService.UpdateSubmit(ctx)
	ctx.Userface.Println(types.GetTime(time.Now()), "Submission", types.ColorizeStatus(ctx.Status))

	// 开始准备评测环境
	ctx.SetStatus("prep_dirs").SetMsg("preparing working directories")
	e.dbService.UpdateSubmit(ctx)

	var submits_dir = path.Join(ctx.Workdir, "submits")
	var workflow_dir = path.Join(ctx.Workdir, "work")

	var rsubmits_dir = path.Join(ctx.RealWorkdir, "submits")
	var rworkflow_dir = path.Join(ctx.RealWorkdir, "work")

	err = os.Mkdir(ctx.Workdir, 0700)
	if err != nil {
		goto workdir_creation_failed
	}
	err = os.Mkdir(submits_dir, 0700)
	if err != nil {
		goto workdir_creation_failed
	}
	err = os.Mkdir(workflow_dir, 0700)
	if err != nil {
		goto workdir_creation_failed
	}
	err = os.Chown(ctx.Workdir, e.cfg.SubmitUid, e.cfg.SubmitGid)
	if err != nil {
		goto workdir_creation_failed
	}
	err = os.Chown(submits_dir, e.cfg.SubmitUid, e.cfg.SubmitGid)
	if err != nil {
		goto workdir_creation_failed
	}
	err = os.Chown(workflow_dir, e.cfg.SubmitUid, e.cfg.SubmitGid)
	if err != nil {
		goto workdir_creation_failed
	}

	defer func() {
		log.Debug().Timestamp().Str("id", ctx.ID).Msg("epilog: chowning workdir to root")
		err = os.Chown(ctx.Workdir, 0, 0)
		if err != nil {
			log.Error().Timestamp().Str("id", ctx.ID).AnErr("err", err).Msg("epilog: failed to chown workdir to root")
		}
		err = os.Chown(submits_dir, 0, 0)
		if err != nil {
			log.Error().Timestamp().Str("id", ctx.ID).AnErr("err", err).Msg("epilog: failed to chown submits_dir to root")
		}
		err = os.Chown(workflow_dir, 0, 0)
		if err != nil {
			log.Error().Timestamp().Str("id", ctx.ID).AnErr("err", err).Msg("epilog: failed to chown workflow_dir to root")
		}
	}()

	goto workdir_created

workdir_creation_failed:
	ctx.SetStatus("failed").SetMsg("failed to create submit workdir")
	e.dbService.UpdateSubmit(ctx)
	return

workdir_created:
	log.Debug().Timestamp().Str("id", ctx.ID).Str("submit_workdir", ctx.Workdir).Msg("created working dirs")

	ctx.Userface.Println(types.GetTime(time.Now()), "Submitting files")

	ctx.SetStatus("prep_files").SetMsg("preparing files")
	e.dbService.UpdateSubmit(ctx)

	for _, submit := range problem.Submits {
		if !submit.IsDir {
			err = e.submitFile(ctx, submits_dir, submit.Path)
			if err != nil {
				ctx.SetStatus("failed").SetMsg("failed to copy submit file " + strconv.Quote(submit.Path))
				e.dbService.UpdateSubmit(ctx)
				ctx.Userface.Println("	*", aurora.Yellow(submit.Path), ":", aurora.Red("failed"))
				return
			}
		} else {
			dir_path := ctx.SubmitDir + "/" + submit.Path
			err = filepath.WalkDir(dir_path, func(path string, info fs.DirEntry, err error) error {
				if err != nil {
					return errors.Wrap(err, "failed to execute filepath.WalkDir")
				}
				if !info.IsDir() {
					if filepath.IsAbs(path) {
						path, _ = filepath.Rel(dir_path, path)
					}
					return e.submitFile(ctx, submits_dir, submit.Path+"/"+path)
				}
				return nil
			})
			if err != nil {
				ctx.SetStatus("failed").SetMsg("failed to copy submit directory " + strconv.Quote(submit.Path))
				e.dbService.UpdateSubmit(ctx)
				ctx.Userface.Println("	*", aurora.Yellow(submit.Path), ":", aurora.Red("failed"))
				return
			}
		}
	}

	log.Debug().Timestamp().Str("id", ctx.ID).Msg("copied submit files")

	ctx.Userface.Println(types.GetTime(time.Now()), "Running Judge workflows")

	ctx.SetStatus("run_workflow").SetMsg("running judge workflows")
	e.dbService.UpdateSubmit(ctx)

	for idx, workflow := range problem.Workflow {
		var _mount = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   submits_dir,
				Target:   "/submits",
				ReadOnly: true,
			},
			{
				Type:   mount.TypeBind,
				Source: workflow_dir,
				Target: "/work",
			},
		}

		var envs = []string{
			"SOJ_SUBMITS_DIR=/submits",
			"SOJ_WORK_DIR=/work",
			"SOJ_REAL_WORKDIR=" + rworkflow_dir,
			"SOJ_REAL_SUBMITDIR=" + rsubmits_dir,
			"SOJ_PROBLEM=" + ctx.Problem,
			"SOJ_SUBMIT=" + ctx.ID,
			"SOJ_WORK_UID=" + strconv.Itoa(e.cfg.SubmitUid),
			"SOJ_WORK_GID=" + strconv.Itoa(e.cfg.SubmitGid),
		}

		for _, mnt := range workflow.Mounts {
			_mount = append(_mount, mount.Mount{
				Type:     mount.Type(mnt.Type),
				Source:   mnt.Source,
				Target:   mnt.Target,
				ReadOnly: mnt.ReadOnly,
			})
		}

		ctx.SetStatus("run_workflow-" + strconv.Itoa(idx))
		e.dbService.UpdateSubmit(ctx)
		ctx.Userface.Println(types.GetTime(time.Now()), "running", "workflow", strconv.Itoa(idx+1), "/", len(problem.Workflow))

		stepshows := map[int]struct{}{}
		stepprivillege := map[int]struct{}{}

		for _, step := range workflow.Show {
			stepshows[step] = struct{}{}
		}
		for _, step := range workflow.PrivilegedSteps {
			stepprivillege[step] = struct{}{}
		}

		var usr = strconv.Itoa(e.cfg.SubmitUid)
		if workflow.Root {
			usr = "0"
		}

		ok, cid := e.docker.RunImage("soj-judge-"+ctx.ID+"-"+strconv.Itoa(idx+1), usr, "soj-judgement", workflow.Image, "/work", _mount, false, false, workflow.DisableNetwork, workflow.Timeout, workflow.NetworkHostMode, envs)

		if !ok {
			ctx.SetStatus("failed").SetMsg("failed to run judge container")
			e.dbService.UpdateSubmit(ctx)
			return
		}

		defer e.docker.CleanContainer(cid)

		steps := make([]types.WorkflowStepResult, len(workflow.Steps))

		for sidx, step := range workflow.Steps {
			ctx.SetStatus("run_workflow-" + strconv.Itoa(idx) + "_" + strconv.Itoa(sidx))
			e.dbService.UpdateSubmit(ctx)

			ctx.Userface.Println(types.GetTime(time.Now()), "running", "workflow", strconv.Itoa(idx+1), "step", strconv.Itoa(sidx+1), "/", len(workflow.Steps))

			_, ok := stepshows[sidx+1]
			_, priv := stepprivillege[sidx+1]

			var rr io.Writer = nil
			var re io.Writer = nil
			if ok {
				ctx.Userface.Println("	$", aurora.Yellow(step))
				rr = &ColoredIO{ctx.Userface, aurora.BlueFg}
				re = &ColoredIO{ctx.Userface, aurora.RedFg}
			}
			ec, logs, err := e.docker.ExecContainer(cid, step, workflow.Timeout, rr, re, envs, priv)

			if ok {
				ctx.Userface.Println(aurora.Gray(15, "exit code:"), aurora.Yellow(ec))
			}

			if ec != 0 || err != nil {
				ctx.SetStatus("failed").SetMsg("failed to run judge " + strconv.Itoa(idx+1) + " step " + strconv.Itoa(sidx+1))
				e.dbService.UpdateSubmit(ctx)

				log.Info().Timestamp().Str("id", ctx.ID).Str("image", workflow.Image).Str("step", step).Int("timeout", workflow.Timeout).AnErr("err", err).Str("logs", logs).Int("exitcode", ec).Msg("failed to run judge step")
				return
			}

			steps[sidx] = types.WorkflowStepResult{
				Logs:     logs,
				ExitCode: ec,
			}

			e.dbService.UpdateSubmit(ctx)
			log.Debug().Timestamp().Str("id", ctx.ID).Str("image", workflow.Image).Str("step", step).Int("timeout", workflow.Timeout).Str("logs", logs).Int("exitcode", ec).Msg("ran judge step")
		}

		logs, err := e.docker.GetContainerLogs(cid)
		if err != nil {
			ctx.SetStatus("failed").SetMsg("failed to get judge logs")
			e.dbService.UpdateSubmit(ctx)
			return
		}

		ctx.WorkflowResults = append(ctx.WorkflowResults, types.WorkflowResult{
			Success: true,
			Logs:    logs,
			Steps:   steps,
		})

		log.Debug().Timestamp().Any("mnt", _mount).Str("id", ctx.ID).Str("image", workflow.Image).Str("logs", logs).Msg("got judge logs")
	}

	ctx.SetStatus("collect_result")
	e.dbService.UpdateSubmit(ctx)

	var result_file = workflow_dir + "/result.json"

	_result, err := os.ReadFile(result_file)

	if err != nil {
		log.Info().Timestamp().Str("id", ctx.ID).Str("result_file", result_file).AnErr("err", err).Msg("failed to read result file")
		ctx.SetStatus("failed").SetMsg("failed to read result file")
		e.dbService.UpdateSubmit(ctx)
		return
	}

	err = json.Unmarshal(_result, &ctx.JudgeResult)
	if err != nil {
		log.Info().Timestamp().Str("id", ctx.ID).Str("result_file", result_file).AnErr("err", err).Msg("failed to parse result file")
		ctx.SetStatus("failed").SetMsg("failed to parse result file")
		e.dbService.UpdateSubmit(ctx)
		return
	}

	ctx.SetStatus("completed").SetMsg("judge successfully finished")
	e.dbService.UpdateSubmit(ctx)
}

// copyFile 复制文件并返回MD5哈希
func (e *Evaluator) copyFile(src, dst string) (string, error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer destinationFile.Close()

	hash := md5.New()
	if _, err = io.Copy(destinationFile, io.TeeReader(sourceFile, hash)); err != nil {
		return "", err
	}

	if err := destinationFile.Sync(); err != nil {
		return "", err
	}

	md5String := hex.EncodeToString(hash.Sum(nil))
	return md5String, nil
}

// submitFile 提交文件到评测环境
func (e *Evaluator) submitFile(ctx *types.SubmitCtx, submits_dir string, submit_path string) error {
	var src_submit_path = path.Join(ctx.SubmitDir, submit_path)
	var dst_submit_path = path.Join(submits_dir, submit_path)

	os.MkdirAll(path.Dir(dst_submit_path), 0700)
	os.Chown(path.Dir(dst_submit_path), e.cfg.SubmitUid, e.cfg.SubmitGid)

	hash, err := e.copyFile(src_submit_path, dst_submit_path)
	if err != nil {
		return err
	} else {
		os.Chown(dst_submit_path, e.cfg.SubmitUid, e.cfg.SubmitGid)
		os.Chmod(dst_submit_path, 0400)

		log.Debug().Timestamp().Str("id", ctx.ID).Str("submit_file", submit_path).Str("hash", hash).Msg("copied submit file")

		ctx.SubmitsHashes = append(ctx.SubmitsHashes, types.SubmitHash{
			Hash: hash,
			Path: submit_path,
		})

		ctx.Userface.Println("	*", aurora.Yellow(submit_path), ":", aurora.Blue(hash))
	}

	return nil
}

// ColoredIO 彩色IO包装器
type ColoredIO struct {
	io.Writer
	aurora.Color
}

func (c *ColoredIO) Write(p []byte) (n int, err error) {
	_, err = c.Writer.Write([]byte(aurora.Colorize(string(p), c.Color).String()))
	return len(p), err
}
