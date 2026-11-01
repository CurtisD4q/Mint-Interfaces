package main

// The blended choice of interface group (--interface blend):
//
//	score(g) = y·A(g) + (1−y)·K(g)
//
// A(g) is how likely the group's rule holds given the program's answers
// (answer probabilities, see answerProbs); K(g) is how likely the group is
// given the program's subject: the keyword scorer's archetype scores, made
// into probabilities (archetypeProbs) and carried over to the groups by
// the archetype-to-group bridge (groupBridge). y is --answer-weight.
//
// The probabilities and the bridge are first estimates, from the
// 1,076-program study's detected answers; they are to be rebuilt from
// checked answers, and y tuned by cross-validation.

import (
	"fmt"
	"io"
	"math"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// InterfaceBlend is the --interface value for the blended choice.
const InterfaceBlend = "blend"

// blendAnswerWeight is y: the weight of the answers against the keywords.
var blendAnswerWeight = 0.5

// answerBase is how often each answer is yes, in the study.
var answerBase = map[appkit.Question]float64{
	appkit.QKeepsRunning: 0.224, appkit.QChangesThings: 0.189, appkit.QTarget: 0.54, appkit.QTwoInputs: 0.057,
	appkit.QSubcommands: 0.133, appkit.QWritesFile: 0.111, appkit.QTable: 0.269, appkit.QNeedsRoot: 0.091,
	appkit.QCanDestroy: 0.187, appkit.QFullScreen: 0.133,
}

// How likely an answer is to be yes, by what the help showed. Silence is a
// mild sign of no: half the usual rate. To be calibrated on checked answers.
const (
	probStrongYes = 0.9
	probHintYes   = 0.65
	probStrongNo  = 0.08
)

// answerProbs turns answers into how likely each is to be yes.
func answerProbs(a appkit.Answers) appkit.AnswerProbs {
	p := appkit.AnswerProbs{}
	for _, q := range appkit.Questions {
		ans := a[q]
		switch {
		case ans.Yes && ans.Sure == appkit.SureStrong:
			p[q] = probStrongYes
		case ans.Yes:
			p[q] = probHintYes
		case ans.Sure == appkit.SureStrong:
			p[q] = probStrongNo
		case ans.Sure == appkit.SureNone:
			p[q] = answerBase[q]
		default:
			p[q] = answerBase[q] / 2
		}
	}
	return p
}

// archetypeTemperature turns score points into probabilities: a lead of
// 20 points (a clear choice, right about 85% of the time) gives the leader
// about 85% against the runner-up.
const archetypeTemperature = 11.5

// archetypeProbs is how likely each archetype is, from the scorer's
// points: a listed program's archetype is certain.
func archetypeProbs(c ArchetypeChoice) map[appkit.ArchetypeID]float64 {
	if c.Confidence == ConfidenceListed || len(c.Scores) == 0 {
		return map[appkit.ArchetypeID]float64{c.ID: 1}
	}
	top := c.Scores[0].Score
	out := map[appkit.ArchetypeID]float64{}
	sum := 0.0
	for _, s := range c.Scores {
		w := math.Exp((s.Score - top) / archetypeTemperature)
		out[s.ID] += w
		sum += w
	}
	for k := range out {
		out[k] /= sum
	}
	return out
}

// keywordGroupProbs is K(g): the archetype probabilities carried over to
// the groups by the bridge.
func keywordGroupProbs(c ArchetypeChoice) map[string]float64 {
	out := map[string]float64{}
	for a, pa := range archetypeProbs(c) {
		row, ok := groupBridge[a]
		if !ok {
			row = groupBridgePrior
		}
		for g, pg := range row {
			out[g] += pa * pg
		}
	}
	return out
}

// blendChoice ranks the groups for a program.
func blendChoice(a appkit.Answers, c ArchetypeChoice, y float64) []appkit.GroupScore {
	return appkit.BlendGroups(answerProbs(a), keywordGroupProbs(c), y)
}

// explainBlend prints the top groups of a blended choice, for --explain.
func explainBlend(w io.Writer, ranked []appkit.GroupScore) {
	if len(ranked) == 0 {
		return
	}
	fmt.Fprintf(w, "  Blend:     answers weight %.2f; lead %.3f over the next group\n", blendAnswerWeight, ranked[0].Score-ranked[1].Score)
	for i, g := range ranked {
		if i == 3 {
			break
		}
		fmt.Fprintf(w, "    %-30s score %.3f  (answers %.3f, keywords %.3f)\n", g.Group.Name, g.Score, g.Answers, g.Words)
	}
}
