package runner

import "github.com/CurtisD4q/Mint-Interfaces/mint"

// JobScreen is a ready-made page for a running job: a Stop action, a
// progress bar (when the job has a Progress parser), and its output, with
// the job's state in the footer. Push it right after Start:
//
//	job := runner.Start(runner.Cmd{Name: "rsync", Args: args, Progress: runner.Percent})
//	ctx.Push(runner.JobScreen("Sync", job))
//
// Leaving the page does not stop the job.
func JobScreen(title string, j *Job) *mint.Screen {
	rows := []mint.Row{
		mint.Action(mint.ActionOpts{Label: "Stop", Get: func() string {
			if j.Running() {
				return "stop"
			}
			return j.State().String()
		}, OnActivate: func(c *mint.Ctx) {
			if j.Running() {
				j.Stop()
			}
		}}),
	}
	if j.c.Progress != nil {
		rows = append(rows, mint.Progress(mint.ProgressOpts{Label: "Progress", Get: j.Percent}))
	}
	rows = append(rows, mint.Log(mint.LogOpts{Label: "Output", Lines: j.Lines, Height: 10}))
	return &mint.Screen{
		Title:    title,
		Sections: []mint.Section{{Rows: rows}},
		Poll:     func(*mint.Screen) bool { return j.Changed() },
		Status:   j.Badges,
	}
}
