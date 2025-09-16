package ui

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ssh "github.com/gliderlabs/ssh"
	"github.com/logrusorgru/aurora/v4"
	"github.com/mrhaoxx/SOJ/types"
)

// SSHHandler SSH处理器
type SSHHandler struct {
	dbService *types.DatabaseService
	cfg       *types.Config
	problems  map[string]types.Problem
	paused    bool
}

// NewSSHHandler 创建新的SSH处理器
func NewSSHHandler(dbService *types.DatabaseService, cfg *types.Config, problems map[string]types.Problem) *SSHHandler {
	return &SSHHandler{
		dbService: dbService,
		cfg:       cfg,
		problems:  problems,
		paused:    false,
	}
}

// SetPaused 设置暂停状态
func (sh *SSHHandler) SetPaused(paused bool) {
	sh.paused = paused
}

// UpdateProblems 更新问题列表
func (sh *SSHHandler) UpdateProblems(problems map[string]types.Problem) {
	sh.problems = problems
}

// HandleSession 处理SSH会话
func (sh *SSHHandler) HandleSession(s ssh.Session) {
	uf := types.Userface{
		Buffer: bytes.NewBuffer(nil),
		Writer: s,
	}

	cmds := s.Command()

	if len(cmds) == 0 {
		uf.Println("Welcome to", aurora.Bold("SOJ"), aurora.Gray(aurora.GrayIndex(10), "Secure Online Judge"), ",", aurora.BrightBlue(s.User()))
		uf.Println(aurora.Yellow(time.Now().Format(time.DateTime + " MST")))
		uf.Println("Use 'submit", aurora.Gray(15, "(sub)"), "<problem_id>' to submit a problem")
		uf.Println("Use 'list", aurora.Gray(15, "(ls)"), "[page]' to list your submissions")
		uf.Println("Use 'status", aurora.Gray(15, "(st)"), "<submit_id>' to show a submission", aurora.Magenta("(fuzzy match)"))
		uf.Println("Use 'rank", aurora.Gray(15, "(rk)"), "' to show rank list")
		uf.Println("Use 'my' to show your submission summary")
		uf.Println("Use 'token' to get token for frontend authentication")
		uf.Println()

	} else {
		uf.Println(aurora.Yellow(time.Now().Format(time.DateTime + " MST")))

		switch cmds[0] {
		case "rank", "rk":
			sh.handleRank(uf)

		case "submit", "sub":
			sh.handleSubmit(s, uf, cmds)

		case "list", "ls":
			sh.handleList(s, uf, cmds)

		case "status", "st":
			sh.handleStatus(s, uf, cmds)

		case "my":
			sh.handleMy(s, uf)

		case "token":
			sh.handleToken(s, uf)

		case "adm":
			sh.handleAdmin(s, uf, cmds)

		default:
			s.Write([]byte("unknown command " + strconv.Quote(s.RawCommand()) + "\n"))
		}
	}
}

// handleRank 处理排行榜命令
func (sh *SSHHandler) handleRank(uf types.Userface) {
	users, err := sh.dbService.GetAllUsersOrderedByScore()
	if err != nil {
		uf.Println(aurora.Red("error:"), "failed to get user rankings")
		return
	}

	var prblmss []string
	for k := range sh.problems {
		prblmss = append(prblmss, k)
	}

	sort.Strings(prblmss)

	var ranks []string

	var cursoc float64 = -1
	var currk int = 0
	for i := range users {
		if users[i].TotalScore != cursoc {
			currk = i
			cursoc = users[i].TotalScore
		}
		ranks = append(ranks, strconv.Itoa(currk+1))

	}

	var userss []string
	for _, u := range users {
		userss = append(userss, u.ID)
	}

	var totalscores []string
	for _, u := range users {
		totalscores = append(totalscores, fmt.Sprintf("%.2f", u.TotalScore))
	}

	var bestscores [][]string

	for _, p := range prblmss {
		var scores []string
		for _, u := range users {
			scores = append(scores, fmt.Sprintf("%.2f", u.BestScores[p]))
		}
		bestscores = append(bestscores, scores)
	}

	var colc = make([]aurora.Color, len(prblmss))
	for i := range colc {
		colc[i] = aurora.WhiteFg | aurora.UnderlineFm
	}

	sh.mkTable(uf, append([]string{"Rank", "User", "Total"}, prblmss...), append([]aurora.Color{aurora.BoldFm | aurora.YellowFg, aurora.BoldFm | aurora.WhiteFg, aurora.BoldFm | aurora.GreenFg}, colc...), append([][]string{ranks, userss, totalscores}, bestscores...))
}

// handleSubmit 处理提交命令
func (sh *SSHHandler) handleSubmit(s ssh.Session, uf types.Userface, cmds []string) {
	if len(cmds) != 2 {
		uf.Println(aurora.Red("error:"), "invalid arguments")
		uf.Println("usage: submit <problem_id>")
		return
	}
	if sh.paused {
		uf.Println(aurora.Red("error:"), "submit is paused. Please try again later")
		return
	}

	// 这里需要调用评测功能，由于重构需要，暂时返回提示
	uf.Println("Submit functionality will be implemented in main.go using judge module")
}

// handleList 处理列表命令
func (sh *SSHHandler) handleList(s ssh.Session, uf types.Userface, cmds []string) {
	if len(cmds) > 2 {
		uf.Println(aurora.Red("error:"), "invalid arguments")
		uf.Println("usage: list [page]")
		return
	}

	uf.Println(aurora.Green("Listing"), aurora.Bold("submissions"))

	page := 1

	if len(cmds) == 2 {
		var err error
		page, err = strconv.Atoi(cmds[1])
		if err != nil {
			uf.Println(aurora.Red("error:"), "invalid page number")
			return
		}
	}

	submits, total, err := sh.dbService.GetSubmitsByUser(s.User(), page, 10)
	if err != nil {
		uf.Println(aurora.Red("error:"), "failed to get submissions")
		return
	}

	uf.Println(aurora.Cyan("Page"), aurora.Bold(page), "of", aurora.Yellow(total/10+1))

	sh.listSubs(uf, submits)
}

// handleStatus 处理状态命令
func (sh *SSHHandler) handleStatus(s ssh.Session, uf types.Userface, cmds []string) {
	if len(cmds) != 2 {
		uf.Println(aurora.Red("error:"), "invalid arguments")
		uf.Println("usage: status <submit_id>")
		return
	}

	uf.Println(aurora.Green("Showing"), aurora.Bold("submission"), aurora.Magenta(cmds[1]))

	submit, err := sh.dbService.FindSubmitsByUserAndPattern(s.User(), cmds[1])
	if err != nil {
		uf.Println(aurora.Red("error:"), "submit", aurora.Yellow(strconv.Quote(cmds[1])), "not found")
		return
	}

	uf.Println()

	sh.showSub(uf, *submit)
}

// handleMy 处理个人信息命令
func (sh *SSHHandler) handleMy(s ssh.Session, uf types.Userface) {
	uf.Println("User", aurora.Bold(aurora.BrightWhite(s.User())))

	user, err := sh.dbService.GetUserByID(s.User())
	if err != nil {
		uf.Println(aurora.Gray(15, "No submissions yet"))
		return
	}

	var prblmss []string
	for k := range sh.problems {
		prblmss = append(prblmss, k)
	}

	sort.Strings(prblmss)

	Cols := []string{"Problem", "Score", "Weight", "Submit ID", "Date"}
	var ColLongest = make([]int, len(Cols))
	for i, col := range Cols {
		ColLongest[i] = len(col)
	}

	var map_succ map[string]bool = make(map[string]bool)

	for _, problem_id := range prblmss {
		sco, ok := user.BestScores[problem_id]
		if ok {
			map_succ[problem_id] = true
		}
		ColLongest[0] = max(ColLongest[0], len(problem_id))
		ColLongest[1] = max(ColLongest[1], len(fmt.Sprintf("%.2f", sco/sh.problems[problem_id].Weight)))
		ColLongest[2] = max(ColLongest[2], len(fmt.Sprintf("%.2f", sh.problems[problem_id].Weight)))
		ColLongest[3] = max(ColLongest[3], len(user.BestSubmits[problem_id]))
		ColLongest[4] = max(ColLongest[4], len(time.Unix(0, user.BestSubmitDate[problem_id]).Format(time.DateTime+" MST")))
	}

	for i, col := range Cols {
		uf.Printf("%-*s ", ColLongest[i], col)
	}

	uf.Println()
	for _, problem_id := range prblmss {
		uf.Printf("%-*s %-*.2f %-*.2f %-*s %-*s\n",
			ColLongest[0], aurora.Bold(aurora.Italic(problem_id)),
			ColLongest[1], aurora.Bold(types.ColorizeScore(types.JudgeResult{Success: map_succ[problem_id], Score: user.BestScores[problem_id] / sh.problems[problem_id].Weight})),
			ColLongest[2], aurora.Bold(sh.problems[problem_id].Weight),
			ColLongest[3], aurora.Magenta(user.BestSubmits[problem_id]),
			ColLongest[4],
			func() aurora.Value {
				if map_succ[problem_id] {
					return aurora.Yellow(time.Unix(0, user.BestSubmitDate[problem_id]).Format(time.DateTime + " MST"))
				} else {
					return aurora.Gray(15, "N/A")
				}
			}())
	}

	uf.Println()
	uf.Println("Total Score:", aurora.Bold(aurora.BrightWhite(user.TotalScore)))
}

// handleToken 处理token命令
func (sh *SSHHandler) handleToken(s ssh.Session, uf types.Userface) {
	user, err := sh.dbService.GetUserByID(s.User())
	if err != nil {
		uf.Println(aurora.Red("error:"), "failed to get user token")
		return
	}
	uf.Println("Your token is:", aurora.Bold(user.Token), "please keep it secret")
}

// handleAdminUserSummary 处理管理员查看用户摘要命令
func (sh *SSHHandler) handleAdminUserSummary(uf types.Userface, targetUser string) {
	uf.Println("User", aurora.Bold(aurora.BrightWhite(targetUser)))

	user, err := sh.dbService.GetUserByID(targetUser)
	if err != nil {
		uf.Println(aurora.Red("error:"), "user not found or no submissions yet")
		return
	}

	var prblmss []string
	for k := range sh.problems {
		prblmss = append(prblmss, k)
	}

	sort.Strings(prblmss)

	Cols := []string{"Problem", "Score", "Weight", "Submit ID", "Date"}
	var ColLongest = make([]int, len(Cols))
	for i, col := range Cols {
		ColLongest[i] = len(col)
	}

	var map_succ map[string]bool = make(map[string]bool)

	for _, problem_id := range prblmss {
		sco, ok := user.BestScores[problem_id]
		if ok {
			map_succ[problem_id] = true
		}
		ColLongest[0] = max(ColLongest[0], len(problem_id))
		ColLongest[1] = max(ColLongest[1], len(fmt.Sprintf("%.2f", sco/sh.problems[problem_id].Weight)))
		ColLongest[2] = max(ColLongest[2], len(fmt.Sprintf("%.2f", sh.problems[problem_id].Weight)))
		ColLongest[3] = max(ColLongest[3], len(user.BestSubmits[problem_id]))
		ColLongest[4] = max(ColLongest[4], len(time.Unix(0, user.BestSubmitDate[problem_id]).Format(time.DateTime+" MST")))
	}

	for i, col := range Cols {
		uf.Printf("%-*s ", ColLongest[i], col)
	}

	uf.Println()
	for _, problem_id := range prblmss {
		uf.Printf("%-*s %-*.2f %-*.2f %-*s %-*s\n",
			ColLongest[0], aurora.Bold(aurora.Italic(problem_id)),
			ColLongest[1], aurora.Bold(types.ColorizeScore(types.JudgeResult{Success: map_succ[problem_id], Score: user.BestScores[problem_id] / sh.problems[problem_id].Weight})),
			ColLongest[2], aurora.Bold(sh.problems[problem_id].Weight),
			ColLongest[3], aurora.Magenta(user.BestSubmits[problem_id]),
			ColLongest[4],
			func() aurora.Value {
				if map_succ[problem_id] {
					return aurora.Yellow(time.Unix(0, user.BestSubmitDate[problem_id]).Format(time.DateTime + " MST"))
				} else {
					return aurora.Gray(15, "N/A")
				}
			}())
	}

	uf.Println()
	uf.Println("Total Score:", aurora.Bold(aurora.BrightWhite(user.TotalScore)))

	// Additional admin info
	uf.Println("Token:", aurora.Gray(15, user.Token))
}

// handleAdminModifySubmission 处理管理员修改提交命令
func (sh *SSHHandler) handleAdminModifySubmission(uf types.Userface, args []string) {
	if len(args) < 2 {
		uf.Println(aurora.Red("error:"), "invalid arguments")
		uf.Println("usage: adm modify <submit_id> <score> [message]")
		return
	}

	submitID := args[0]
	scoreStr := args[1]

	// Parse score
	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil {
		uf.Println(aurora.Red("error:"), "invalid score:", aurora.Yellow(scoreStr))
		return
	}

	// Get message if provided
	var message string
	if len(args) >= 3 {
		message = strings.Join(args[2:], " ")
	}

	// Get existing submission
	submit, err := sh.dbService.GetSubmitByID(submitID)
	if err != nil {
		uf.Println(aurora.Red("error:"), "submit", aurora.Yellow(strconv.Quote(submitID)), "not found")
		return
	}

	// Store original values for display
	originalScore := submit.JudgeResult.Score
	originalMessage := submit.Msg
	originalSuccess := submit.JudgeResult.Success

	// Update submission with new values
	err = sh.dbService.ModifySubmissionResult(submitID, score, message, sh.problems)
	if err != nil {
		uf.Println(aurora.Red("error:"), "failed to modify submission:", err.Error())
		return
	}

	// Display success message
	uf.Println(aurora.Green("Success:"), "Modified submit", aurora.Magenta(submitID))
	uf.Println("  User:", aurora.Blue(submit.User))
	uf.Println("  Problem:", aurora.Bold(submit.Problem))
	uf.Println("  Score:", aurora.Yellow(fmt.Sprintf("%.2f", originalScore)), "→", aurora.Green(fmt.Sprintf("%.2f", score)))
	if message != "" {
		uf.Println("  Message:", aurora.Gray(15, originalMessage), "→", aurora.Cyan(message))
	}
	if originalSuccess != (score > 0) {
		uf.Println("  Success status changed:", aurora.Yellow(originalSuccess), "→", aurora.Green(score > 0))
	}
	uf.Println("  User records have been updated")
}

// handleAdmin 处理管理员命令
func (sh *SSHHandler) handleAdmin(s ssh.Session, uf types.Userface, cmds []string) {
	if !sh.dbService.IsAdmin(s.User()) {
		s.Write([]byte("unknown command " + strconv.Quote(s.RawCommand()) + "\n"))
		return
	}

	if len(cmds) < 2 {
		uf.Println(aurora.Red("error:"), "invalid arguments")
		uf.Println("usage: adm <command>")
		return
	}
	switch cmds[1] {
	case "list":
		if len(cmds) > 4 {
			uf.Println(aurora.Red("error:"), "invalid arguments")
			uf.Println("usage: adm list [page]")
			uf.Println("       adm list <username> [page]")
			return
		}

		var username string
		page := 1
		var err error

		if len(cmds) >= 3 {
			// Check if the third argument is a number (page) or username
			if pageNum, err := strconv.Atoi(cmds[2]); err == nil {
				// It's a page number, list all submissions
				page = pageNum
			} else {
				// It's a username, list submissions for that user
				username = cmds[2]
				if len(cmds) == 4 {
					page, err = strconv.Atoi(cmds[3])
					if err != nil {
						uf.Println(aurora.Red("error:"), "invalid page number")
						return
					}
				}
			}
		}

		var submits []types.SubmitCtx
		var total int64

		if username != "" {
			// List submissions for specific user
			submits, total, err = sh.dbService.GetSubmitsByUser(username, page, 20)
			if err != nil {
				uf.Println(aurora.Red("error:"), "failed to get submissions for user", aurora.Yellow(username))
				return
			}
			uf.Println(aurora.Green("Listing submissions for user"), aurora.Bold(aurora.Blue(username)))
		} else {
			// List all submissions
			submits, total, err = sh.dbService.GetAllSubmits(page, 20)
			if err != nil {
				uf.Println(aurora.Red("error:"), "failed to get submissions")
				return
			}
			uf.Println(aurora.Green("Listing"), aurora.Bold("all submissions"))
		}

		uf.Println(aurora.Cyan("Page"), aurora.Bold(page), "of", aurora.Yellow(total/20+1))

		sh.listSubs(uf, submits)
	case "status":
		if len(cmds) != 3 {
			uf.Println(aurora.Red("error:"), "invalid arguments")
			uf.Println("usage: adm status <submit_id>")
			return
		}

		uf.Println(aurora.Green("Showing"), aurora.Bold("submission"), aurora.Magenta(cmds[2]))

		submit, err := sh.dbService.GetSubmitByID(cmds[2])
		if err != nil {
			uf.Println(aurora.Red("error:"), "submit", aurora.Yellow(strconv.Quote(cmds[2])), "not found")
			return
		}

		uf.Println()

		sh.showSub(uf, *submit)
	case "pause":
		sh.SetPaused(true)
		uf.Println(aurora.Green("Submit"), aurora.Bold("paused"))
	case "delete":
		if len(cmds) != 3 {
			uf.Println(aurora.Red("error:"), "invalid arguments")
			uf.Println("usage: adm delete <submit_id>")
			return
		}

		submitID := cmds[2]

		// 先获取提交记录信息，用于显示反馈
		submit, err := sh.dbService.GetSubmitByID(submitID)
		if err != nil {
			uf.Println(aurora.Red("error:"), "submit", aurora.Yellow(strconv.Quote(submitID)), "not found")
			return
		}

		// 删除提交记录并更新用户记录
		err = sh.dbService.DeleteSubmitByIDWithProblems(submitID, sh.problems)
		if err != nil {
			uf.Println(aurora.Red("error:"), "failed to delete submit:", err.Error())
			return
		}

		uf.Println(aurora.Green("Success:"), "Deleted submit", aurora.Magenta(submitID))
		uf.Println("  User:", aurora.Blue(submit.User))
		uf.Println("  Problem:", aurora.Bold(submit.Problem))
		uf.Println("  User records have been updated")

	case "user":
		if len(cmds) != 3 {
			uf.Println(aurora.Red("error:"), "invalid arguments")
			uf.Println("usage: adm user <username>")
			return
		}
		sh.handleAdminUserSummary(uf, cmds[2])
	case "modify":
		if len(cmds) < 4 {
			uf.Println(aurora.Red("error:"), "invalid arguments")
			uf.Println("usage: adm modify <submit_id> <score> [message]")
			return
		}
		sh.handleAdminModifySubmission(uf, cmds[2:])
	case "reload":
		// 这个功能需要在main.go中实现
		uf.Println("Reload functionality will be implemented in main.go")
	}
}

// listSubs 列出提交
func (sh *SSHHandler) listSubs(uf types.Userface, submits []types.SubmitCtx) {
	if len(submits) == 0 {
		uf.Println(aurora.Gray(15, "No submissions yet"))
	} else {
		Cols := []string{"ID", "User", "Problem", "Status", "Message", "Score", "Judge Message", "Date"}
		var ColLongest = make([]int, len(Cols))
		for i, col := range Cols {
			ColLongest[i] = len(col)
		}

		for _, submit := range submits {
			ColLongest[0] = max(ColLongest[0], len(submit.ID))
			ColLongest[1] = max(ColLongest[1], len(submit.User))
			ColLongest[2] = max(ColLongest[2], len(submit.Problem))
			ColLongest[3] = max(ColLongest[3], len(submit.Status))
			ColLongest[4] = max(ColLongest[4], len(submit.Msg))
			ColLongest[5] = max(ColLongest[5], len(fmt.Sprintf("%.2f", submit.JudgeResult.Score)))
			ColLongest[6] = max(ColLongest[6], len(sh.omitStr(submit.JudgeResult.Msg, 20)))
			ColLongest[7] = max(ColLongest[7], len(time.Unix(0, submit.SubmitTime).Format(time.DateTime)))
		}

		for i, col := range Cols {
			uf.Printf("%-*s ", ColLongest[i], col)
		}
		uf.Println()

		for _, submit := range submits {
			uf.Printf("%-*s %-*s %-*s %-*s %-*s %-*.2f %-*s %-*s\n",
				ColLongest[0], aurora.Magenta(submit.ID),
				ColLongest[1], aurora.Blue(submit.User),
				ColLongest[2], aurora.Bold(submit.Problem),
				ColLongest[3], types.ColorizeStatus(submit.Status),
				ColLongest[4], aurora.Gray(15, submit.Msg),
				ColLongest[5], types.ColorizeScore(submit.JudgeResult),
				ColLongest[6], aurora.Gray(15, sh.omitStr(submit.JudgeResult.Msg, 20)),
				ColLongest[7], aurora.Yellow(time.Unix(0, submit.SubmitTime).Format(time.DateTime)))
		}
	}
}

// showSub 显示提交详情
func (sh *SSHHandler) showSub(uf types.Userface, submit types.SubmitCtx) {
	uf.Println("Submit ID:", aurora.Magenta(submit.ID))
	uf.Println("User:", aurora.Blue(submit.User))
	uf.Println("Problem:", aurora.Bold(submit.Problem))
	uf.Println("Status:", types.ColorizeStatus(submit.Status))
	uf.Println("Message:", aurora.Gray(15, submit.Msg))
	uf.Println("Submit Time:", aurora.Yellow(time.Unix(0, submit.SubmitTime).Format(time.DateTime+" MST")))

	if submit.Status == "completed" {
		if submit.JudgeResult.Success {
			uf.Printf("Score %.2f %s\n", aurora.Underline(aurora.Bold(types.ColorizeScore(submit.JudgeResult))), aurora.Italic(aurora.Gray(15, "max.100 (Unweighted)")))
		} else {
			uf.Println(aurora.Red("Judgement is Failed"))
		}

		uf.Println("Judgement Message:")

		if len(submit.JudgeResult.Msg) > 0 {
			uf.Println(aurora.Bold(aurora.Cyan("	" + strings.ReplaceAll(submit.JudgeResult.Msg, "\n", "\n	"))))
		} else {
			uf.Println("	", aurora.Gray(15, "No message"))
		}
	} else {
		uf.Println(aurora.Italic(aurora.Underline(aurora.Bold(aurora.Gray(15, "No judgement result")))))
	}
	uf.Println()

	uf.Println("Logs:")
	uf.Write(submit.Userface.Buffer.Bytes())

	uf.Println()
}

// mkTable 创建表格
func (sh *SSHHandler) mkTable(uf types.Userface, cols []string, colc []aurora.Color, data [][]string) {
	var ColLongest = make([]int, len(cols))
	for i, col := range cols {
		ColLongest[i] = len(col)
	}

	for i := 0; i < len(data[0]); i++ {
		for j := 0; j < len(cols); j++ {
			ColLongest[j] = max(ColLongest[j], len(data[j][i]))
		}
	}

	for i, col := range cols {
		uf.Printf("%-*s ", ColLongest[i], col)
	}
	uf.Println()

	// for _, row := range data {
	// 	for i, col := range row {
	// 		uf.Printf("%-*s ", ColLongest[i], col)
	// 	}
	// 	uf.Println()
	// }
	for i := 0; i < len(data[0]); i++ {
		for j := 0; j < len(cols); j++ {
			uf.Printf("%-*s ", ColLongest[j], aurora.Colorize(data[j][i], colc[j]))
		}
		uf.Println()
	}
}

// omitStr 省略字符串
func (sh *SSHHandler) omitStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
