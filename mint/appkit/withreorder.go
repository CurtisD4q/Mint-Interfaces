package appkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/reorder"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// MixinReorder changes the order of firewall and routing-policy rules,
// with a review of the exact commands and an automatic way back.
const MixinReorder MixinName = "WithReorder"

// ReorderSpec configures WithReorder.
type ReorderSpec struct {
	// Adapter is the tool: iptables, ip6tables (or an -nft/-legacy
	// variant), nft, ufw or ip-rule. ip-route is refused: routes have no order.
	Adapter string `json:"adapter"`
	// Scope is the chain to open on ("filter INPUT", "inet filter input",
	// "-4"); empty opens on the first.
	Scope string `json:"scope,omitempty"`
	// ConfirmWindow is how long the user has to keep a new order before
	// Mint puts the old one back. Default 30 s; 10 s to 10 min.
	ConfirmWindow time.Duration `json:"confirm_window,omitempty"`
}

// KnownReorderSpec returns WithReorder's settings for a tool Mint knows.
func KnownReorderSpec(bin string) (ReorderSpec, bool) {
	b := toolBase(bin)
	switch {
	case b == "nft", b == "ufw":
		return ReorderSpec{Adapter: b}, true
	case strings.HasPrefix(b, "iptables"), strings.HasPrefix(b, "ip6tables"):
		if _, err := reorder.For(b); err == nil {
			s := ReorderSpec{Adapter: b}
			if b == "iptables" || b == "ip6tables" {
				s.Scope = "filter INPUT"
			}
			return s, true
		}
	}
	return ReorderSpec{}, false
}

func validateReorder(s *ToolSpec) error {
	if _, err := reorder.For(s.Reorder.Adapter); err != nil {
		return err
	}
	if w := s.Reorder.ConfirmWindow; w != 0 && (w < 10*time.Second || w > 10*time.Minute) {
		return fmt.Errorf("confirm_window must be between 10s and 10m, got %v", w)
	}
	return nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinReorder, Order: 107,
		Summary:  "change the order of rules, review the commands, and go back if not kept (Rules: iptables, nft, ufw, ip rule)",
		Fields:   "Reorder *ReorderSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Reorder != nil },
		Validate: validateReorder,
	})
}

// reorderState is what the rule order screen shows. Guarded by a.mu.
type reorderState struct {
	ad     reorder.Adapter
	run    reorder.Runner
	scopes []string
	scope  string
	before reorder.List // as last read
	order  []int        // staged: display position -> index in before
	err    string

	pending *pendingOrder // the last change, until kept or put back
}

// pendingOrder is an applied change waiting to be kept.
type pendingOrder struct {
	original reorder.List
	deadline time.Time
	timer    *time.Timer
	backup   string // where the rules before the change were saved
	result   string // "" until kept or put back
	failed   bool   // putting the old order back failed
}

func (a *ToolApp) confirmWindow() time.Duration {
	if w := a.Spec.Reorder.ConfirmWindow; w > 0 {
		return w
	}
	return 30 * time.Second
}

func (a *ToolApp) reorder() *reorderState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rules == nil {
		ad, err := reorder.For(a.Spec.Reorder.Adapter)
		a.rules = &reorderState{ad: ad, run: reorder.Exec}
		if err != nil {
			a.rules.err = err.Error()
		}
	}
	return a.rules
}

// reorderLoad reads the rules of scope ("" for the spec's, or the first),
// dropping staged moves.
func (a *ToolApp) reorderLoad(ctx context.Context, scope string) error {
	st := a.reorder()
	if st.ad == nil {
		return errors.New(st.err)
	}
	a.mu.Lock()
	scopes := st.scopes
	a.mu.Unlock()
	if scopes == nil {
		s, err := st.ad.Scopes(ctx, st.run)
		if err != nil {
			a.setReorderErr(err)
			return err
		}
		scopes = s
	}
	if scope == "" {
		scope = a.Spec.Reorder.Scope
	}
	if scope == "" && len(scopes) > 0 {
		scope = scopes[0]
	}
	l, err := st.ad.List(ctx, st.run, scope)
	a.mu.Lock()
	defer a.mu.Unlock()
	st.scopes = scopes
	if err != nil {
		st.err = err.Error()
		return err
	}
	st.scope, st.before, st.err = scope, l, ""
	st.order = make([]int, len(l.Rules))
	for i := range st.order {
		st.order[i] = i
	}
	return nil
}

func (a *ToolApp) setReorderErr(err error) {
	a.mu.Lock()
	a.rules.err = err.Error()
	a.mu.Unlock()
}

// reorderMove moves the rule at display position from to position to.
func (a *ToolApp) reorderMove(from, to int) error {
	st := a.reorder()
	a.mu.Lock()
	defer a.mu.Unlock()
	n := len(st.order)
	if from < 0 || from >= n || to < 0 || to >= n {
		return errors.New("no rule there")
	}
	if from == to {
		return nil
	}
	next := append([]int(nil), st.order...)
	v := next[from]
	next = append(next[:from], next[from+1:]...)
	next = append(next[:to], append([]int{v}, next[to:]...)...)
	if _, err := reorder.Permute(st.before, next); err != nil {
		return err
	}
	st.order = next
	return nil
}

// movableRange is the first and last display position a rule can take.
func (st *reorderState) movableRange() (lo, hi int) {
	lo, hi = 0, len(st.order)-1
	for lo <= hi && st.before.Rules[st.order[lo]].Fixed {
		lo++
	}
	for hi >= lo && st.before.Rules[st.order[hi]].Fixed {
		hi--
	}
	return lo, hi
}

// reorderPlan re-reads the rules and returns the plan for the staged
// order. reorder.ErrChanged means someone else changed them.
func (a *ToolApp) reorderPlan(ctx context.Context) (reorder.Plan, error) {
	st := a.reorder()
	a.mu.Lock()
	before, order := st.before, append([]int(nil), st.order...)
	a.mu.Unlock()
	_, p, err := reorder.Prepare(ctx, st.ad, st.run, before, order)
	return p, err
}

// reorderApply applies the staged order. The rules before it are saved to
// a file first; afterwards the user has confirmWindow to keep the new
// order, or the old one comes back by itself.
func (a *ToolApp) reorderApply(ctx context.Context) error {
	st := a.reorder()
	a.mu.Lock()
	if p := st.pending; p != nil && p.result == "" {
		a.mu.Unlock()
		return errors.New("keep or undo the last change first")
	}
	before, order := st.before, append([]int(nil), st.order...)
	a.mu.Unlock()
	if !reorder.Moved(order) {
		return errors.New("nothing has moved")
	}
	fresh, p, err := reorder.Prepare(ctx, st.ad, st.run, before, order)
	if err != nil {
		if errors.Is(err, reorder.ErrChanged) {
			_ = a.reorderLoad(ctx, before.Scope)
		}
		return err
	}
	backup, err := a.saveRulesBackup(fresh)
	if err != nil {
		return fmt.Errorf("couldn't save a copy of the rules first, so nothing was changed: %v", err)
	}
	if err := reorder.Apply(ctx, st.ad, st.run, fresh, p); err != nil {
		_ = a.reorderLoad(ctx, before.Scope)
		return fmt.Errorf("%v (a copy of the rules before is in %s)", err, backup)
	}
	pend := &pendingOrder{original: fresh, deadline: time.Now().Add(a.confirmWindow()), backup: backup}
	pend.timer = time.AfterFunc(a.confirmWindow(), func() {
		if err := a.reorderRevert(context.Background(), "not kept in time"); err == nil {
			mint.ToastWarn("The rules are back in their old order: the new one wasn't kept in time.")
		}
	})
	_ = a.reorderLoad(ctx, before.Scope)
	a.mu.Lock()
	st.pending = pend
	a.mu.Unlock()
	return nil
}

// reorderKeep keeps the new order.
func (a *ToolApp) reorderKeep() bool {
	st := a.reorder()
	a.mu.Lock()
	defer a.mu.Unlock()
	p := st.pending
	if p == nil || p.result != "" {
		return false
	}
	p.timer.Stop()
	p.result = "Kept the new order."
	return true
}

// reorderRevert puts the order from before the last change back, unless
// it was kept. why says what made it happen.
func (a *ToolApp) reorderRevert(ctx context.Context, why string) error {
	st := a.reorder()
	a.mu.Lock()
	p := st.pending
	if p == nil || p.result != "" {
		a.mu.Unlock()
		return errors.New("there is no change to undo")
	}
	p.timer.Stop()
	p.result = "Putting the old order back…"
	a.mu.Unlock()

	err := reorder.Restore(ctx, st.ad, st.run, p.original)
	a.mu.Lock()
	if err != nil {
		p.failed = true
		p.result = "Couldn't put the old order back (" + err.Error() + "). The rules before the change are saved in " + p.backup + "."
	} else {
		p.result = "The old order is back (" + why + ")."
	}
	scope := p.original.Scope
	a.mu.Unlock()
	_ = a.reorderLoad(ctx, scope)
	if err != nil {
		mint.ToastError("Couldn't put the old rule order back; see the rule order screen")
	}
	return err
}

// reorderPending returns the seconds left to keep a change, or -1.
func (a *ToolApp) reorderPending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rules == nil || a.rules.pending == nil || a.rules.pending.result != "" {
		return -1
	}
	return max(0, int(time.Until(a.rules.pending.deadline).Round(time.Second)/time.Second))
}

// closeReorder puts the old order back if the app is closing before the
// user kept a change.
func (a *ToolApp) closeReorder() {
	if a.reorderPending() >= 0 {
		_ = a.reorderRevert(context.Background(), "the app closed before it was kept")
	}
}

// saveRulesBackup writes l to the data folder (mode 0600) and returns
// the path.
func (a *ToolApp) saveRulesBackup(l reorder.List) (string, error) {
	dir := filepath.Join(a.DataDir, "rule-backups")
	if err := util.Mkdir(dir); err != nil {
		return "", err
	}
	base := fmt.Sprintf("%s-%s-%s", l.Tool, strings.NewReplacer(" ", "_", "/", "_").Replace(strings.TrimLeft(l.Scope, "-")), time.Now().Format("20060102-150405"))
	var f *os.File
	var path string
	var err error
	for n := 1; n <= 100; n++ {
		name := base + ".txt"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.txt", base, n)
		}
		path = filepath.Join(dir, name)
		// Never over an existing file, or through a link someone left there.
		if f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); !errors.Is(err, os.ErrExist) {
			break
		}
	}
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(reorder.Backup(l)); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	a.giveBack(dir, path)
	return path, nil
}

// movedRules returns the rules (indexes into before) that moved: those
// outside the longest run that kept its order. Moving rule 4 to the top
// moves one rule, not four.
func movedRules(order []int) map[int]bool {
	// Longest increasing subsequence, O(n log n), with back links.
	n := len(order)
	tails, prev := []int{}, make([]int, n)
	for i, v := range order {
		lo, hi := 0, len(tails)
		for lo < hi {
			m := (lo + hi) / 2
			if order[tails[m]] < v {
				lo = m + 1
			} else {
				hi = m
			}
		}
		prev[i] = -1
		if lo > 0 {
			prev[i] = tails[lo-1]
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	kept := make([]bool, n)
	if len(tails) > 0 {
		for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
			kept[i] = true
		}
	}
	moved := map[int]bool{}
	for i, v := range order {
		if !kept[i] {
			moved[v] = true
		}
	}
	return moved
}

// tildePath shortens a path in the home folder to ~/….
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "/" && strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func ruleLabel(i int, r reorder.Rule) string {
	return fmt.Sprintf("%2d  %s", i+1, mint.CleanLineVisible(r.Text))
}

// ReorderScreen lists the rules of one chain. Choosing a rule moves it;
// moves are only staged until reviewed and applied.
func (a *ToolApp) ReorderScreen() *mint.Screen {
	st := a.reorder()
	scr := &mint.Screen{Title: "Rule order"}
	loaded := false
	scr.Enter = func(*mint.Screen) {
		if !loaded {
			loaded = true
			_ = a.reorderLoad(context.Background(), "")
		}
	}
	lastLeft := -2
	scr.Poll = func(*mint.Screen) bool {
		left := a.reorderPending()
		changed := left != lastLeft
		lastLeft = left
		return changed
	}
	scr.Status = func() []mint.Badge {
		b := a.statusBadges()
		if left := a.reorderPending(); left >= 0 {
			b = append([]mint.Badge{{Text: fmt.Sprintf("old order back in %d s", left), Kind: mint.BadgeWarn}}, b...)
		}
		return b
	}
	scr.Refresh = func(s *mint.Screen) {
		a.mu.Lock()
		before, order, scope, errText, pend := st.before, append([]int(nil), st.order...), st.scope, st.err, st.pending
		var pendCopy pendingOrder
		if pend != nil {
			pendCopy = *pend
		}
		a.mu.Unlock()
		s.Banner = ""
		if !runningAsRoot() {
			s.Banner = "Reading and changing " + a.Spec.Name + " rules needs root. Mint never asks for your password: start the app with sudo."
		}
		var secs []mint.Section
		if pend != nil {
			secs = append(secs, a.pendingSection(pendCopy))
		}
		secs = append(secs, mint.Section{Name: "Chain", Rows: []mint.Row{mint.Action(mint.ActionOpts{
			ID: "reorder:scope", Label: "Rules of", Get: func() string { return scope },
			OnActivate: func(c *mint.Ctx) { a.chooseScope(c) },
		})}})
		if errText != "" {
			secs = append(secs, mint.Section{Name: "Problem", Rows: []mint.Row{mint.Note(mint.CleanLineVisible(errText))}})
			s.Sections = secs
			return
		}
		var rows []mint.Row
		movedSet := movedRules(order)
		moved := len(movedSet)
		for i, j := range order {
			i, r := i, before.Rules[j]
			mark := "move"
			switch {
			case r.Fixed:
				mark = "fixed"
			case movedSet[j]:
				mark = fmt.Sprintf("was %d", j+1)
			}
			rows = append(rows, mint.Action(mint.ActionOpts{
				ID: "reorder:rule:" + strconv.Itoa(j), Label: ruleLabel(i, r), Get: func() string { return mark },
				OnActivate: func(c *mint.Ctx) { a.chooseMove(c, i) },
			}))
		}
		if len(rows) == 0 {
			rows = append(rows, mint.Note("No rules here."))
		}
		secs = append(secs, mint.Section{Name: fmt.Sprintf("Rules, first to last (%d)", len(rows)), Rows: rows})
		var act []mint.Row
		if moved > 0 {
			act = append(act,
				mint.Action(mint.ActionOpts{ID: "reorder:review", Label: fmt.Sprintf("Review and apply (%d moved)…", moved),
					OnActivate: func(c *mint.Ctx) { a.reviewOrder(c) }}),
				mint.Action(mint.ActionOpts{ID: "reorder:undo", Label: "Undo the moves",
					OnActivate: func(c *mint.Ctx) { _ = a.reorderLoad(context.Background(), scope) }}),
			)
		} else {
			act = append(act, mint.Note("Choose a rule to move it. Nothing changes until you review and apply."))
		}
		act = append(act, mint.Action(mint.ActionOpts{ID: "reorder:reload", Label: "Read the rules again",
			OnActivate: func(c *mint.Ctx) {
				c.Busy("Reading the rules…", func() { _ = a.reorderLoad(context.Background(), scope) })
			}}))
		secs = append(secs, mint.Section{Name: "Apply", Rows: act})
		s.Sections = secs
	}
	return scr
}

func (a *ToolApp) pendingSection(p pendingOrder) mint.Section {
	if p.result != "" {
		rows := []mint.Row{mint.Note(p.result)}
		if !p.failed {
			rows = append(rows, mint.Note("A copy of the rules from before the change:"), mint.Note("  "+tildePath(p.backup)))
		}
		return mint.Section{Name: "Last change", Rows: rows}
	}
	left := max(0, int(time.Until(p.deadline).Round(time.Second)/time.Second))
	return mint.Section{Name: "Keep the new order?", Rows: []mint.Row{
		mint.Note(fmt.Sprintf("The new order is in place. Unless you keep it, the old order comes back in %d s,", left)),
		mint.Note("so a change that cuts you off undoes itself."),
		mint.Action(mint.ActionOpts{ID: "reorder:keep", Label: "Keep the new order",
			OnActivate: func(c *mint.Ctx) {
				if a.reorderKeep() {
					c.Toast("Kept")
				}
			}}),
		mint.Action(mint.ActionOpts{ID: "reorder:revert", Label: "Put the old order back now",
			OnActivate: func(c *mint.Ctx) {
				var err error
				c.Busy("Putting the old order back…", func() { err = a.reorderRevert(context.Background(), "you asked") })
				if err == nil {
					c.Toast("The old order is back")
				}
			}}),
	}}
}

func (a *ToolApp) chooseScope(c *mint.Ctx) {
	st := a.reorder()
	a.mu.Lock()
	scopes, moved := st.scopes, reorder.Moved(st.order)
	a.mu.Unlock()
	if len(scopes) < 2 {
		c.ToastWarn("There is only one list of rules here")
		return
	}
	items := make([]mint.Choice, len(scopes))
	for i, s := range scopes {
		items[i] = mint.Choice{Label: s, Value: s}
	}
	ch, ok := c.ChooseItem("Which rules?", items, "")
	if !ok {
		return
	}
	if moved && !c.Confirm("Drop the moves?", "The rules you moved here haven't been applied. Show other rules and drop the moves?") {
		return
	}
	c.Busy("Reading the rules…", func() { _ = a.reorderLoad(context.Background(), ch.Value) })
}

func (a *ToolApp) chooseMove(c *mint.Ctx, i int) {
	st := a.reorder()
	a.mu.Lock()
	r := st.before.Rules[st.order[i]]
	lo, hi := st.movableRange()
	a.mu.Unlock()
	if r.Fixed {
		c.ToastWarn("This rule stays where it is: the system depends on it")
		return
	}
	var items []mint.Choice
	add := func(label string, to int) {
		if to != i && to >= lo && to <= hi {
			items = append(items, mint.Choice{Label: label, Value: strconv.Itoa(to)})
		}
	}
	add("Up one", i-1)
	add("Down one", i+1)
	add("To the top", lo)
	add("To the bottom", hi)
	if hi-lo > 1 {
		items = append(items, mint.Choice{Label: "To position…", Value: "pos"})
	}
	if len(items) == 0 {
		c.ToastWarn("There is nowhere to move it")
		return
	}
	ch, ok := c.ChooseItem("Move rule "+strconv.Itoa(i+1), items, util.Truncate(mint.CleanLineVisible(r.Text), 70))
	if !ok {
		return
	}
	to, err := strconv.Atoi(ch.Value)
	if ch.Value == "pos" {
		v, ok := c.Prompt("Move to position", fmt.Sprintf("A number from %d to %d", lo+1, hi+1), "")
		if !ok {
			return
		}
		to, err = strconv.Atoi(strings.TrimSpace(v))
		to--
	}
	if err != nil || to < lo || to > hi {
		c.ToastError(fmt.Sprintf("Choose a position from %d to %d", lo+1, hi+1))
		return
	}
	if err := a.reorderMove(i, to); err != nil {
		c.ToastError(err.Error())
	}
}

// reviewOrder shows the new order and the exact commands, and applies.
func (a *ToolApp) reviewOrder(c *mint.Ctx) {
	var p reorder.Plan
	var err error
	c.Busy("Working out the commands…", func() { p, err = a.reorderPlan(context.Background()) })
	if err != nil {
		if errors.Is(err, reorder.ErrChanged) {
			_ = a.reorderLoad(context.Background(), "")
			c.Message("The rules changed", "Someone or something changed these rules after Mint read them, so the moves no longer mean what they did. The rules have been read again; move them again.")
			return
		}
		c.ToastError(err.Error())
		return
	}
	st := a.reorder()
	a.mu.Lock()
	before, order := st.before, append([]int(nil), st.order...)
	a.mu.Unlock()
	var newRows []mint.Row
	movedSet := movedRules(order)
	for i, j := range order {
		mark := "  "
		if movedSet[j] {
			mark = "→ "
		}
		newRows = append(newRows, mint.Note(mark+ruleLabel(i, before.Rules[j])))
	}
	how := "All at once: " + a.Spec.Name + " checks the new order, then switches to it in one step."
	if !p.Atomic {
		how = "Step by step: between these commands some rules are briefly missing."
	}
	var cmdRows []mint.Row
	for _, s := range p.Steps {
		cmdRows = append(cmdRows, mint.Note(mint.CleanLineVisible(s.String())))
	}
	window := a.confirmWindow()
	scr := &mint.Screen{Title: "Review the new order", Sections: []mint.Section{
		{Name: "New order", Rows: newRows},
		{Name: "How", Rows: []mint.Row{mint.Note(how),
			mint.Note("Mint saves the current rules to a file first."),
			mint.Note(fmt.Sprintf("Then you have %d s to keep the new order, or the old one comes back.", int(window.Seconds())))}},
		{Name: "Commands", Rows: cmdRows},
		{Name: "Apply", Rows: []mint.Row{mint.Action(mint.ActionOpts{ID: "reorder:apply", Label: "Apply the new order",
			OnActivate: func(c *mint.Ctx) {
				var err error
				c.Busy("Changing the order…", func() { err = a.reorderApply(context.Background()) })
				switch {
				case errors.Is(err, reorder.ErrChanged):
					c.Message("The rules changed", "Someone or something changed these rules after Mint read them. Nothing was changed; the rules have been read again.")
				case err != nil:
					c.Message("The order wasn't changed", mint.CleanLineVisible(err.Error()))
				default:
					c.Toast(fmt.Sprintf("Applied. Keep it within %d s", int(window.Seconds())))
				}
				c.Pop()
			}})}},
	}}
	c.Push(scr)
}
