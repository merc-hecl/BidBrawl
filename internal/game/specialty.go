package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

var specialtyStyles = []string{"quick", "survey", "research", "planner", "adaptive"}
var specialtyTopics = []string{"quality_mix", "upper_mix", "space_mix", "cross_mix", "slender_mix", "compact_mix"}

type SpecialtyQuestion struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SpecialtyStatus is an explicit owner/observer projection. Frozen sample IDs
// and unchosen answers must never be serialized through an observation.
type SpecialtyStatus struct {
	Style       string              `json:"style"`
	Round       int                 `json:"round"`
	Actions     []string            `json:"actions"`
	Questions   []SpecialtyQuestion `json:"questions"`
	Choices     []specialtyChoice   `json:"choices"`
	Reserved    bool                `json:"reserved"`
	Preparation int                 `json:"preparation"`
	BoostArmed  bool                `json:"boost_armed"`
	Research    *ResearchStatus     `json:"research,omitempty"`
}

type ResearchStatus struct {
	Topic     string `json:"topic"`
	Committed bool   `json:"committed"`
	Delivered bool   `json:"delivered"`
	DueRound  int    `json:"due_round"`
}

type specialtyChoice struct {
	Round  int    `json:"round"`
	Action string `json:"action"`
	Topic  string `json:"topic,omitempty"`
}

type specialtyPlan struct {
	Round   int        `json:"round"`
	Topics  []string   `json:"topics"`
	Samples [][]string `json:"samples"`
}

// Only private snapshots contain these fields. A plan is generated on round
// entry, before the player chooses a question; reads never consume randomness.
type specialtyProgress struct {
	Plans       []specialtyPlan   `json:"plans"`
	Choices     []specialtyChoice `json:"choices"`
	Reserved    bool              `json:"reserved"`
	Preparation []int             `json:"preparation"`
	ArmedRound  int               `json:"armed_round"`
	ArmedTopic  string            `json:"armed_topic"`
	Research    *ResearchStatus   `json:"research,omitempty"`
}

func topicName(topic string) string {
	return map[string]string{
		"quality_mix": "稀有及以上品质占比", "upper_mix": "史诗及以上品质占比",
		"space_mix": "面积至少 4 格的占比", "cross_mix": "稀有及以上且面积至少 4 格的占比",
		"slender_mix": "宽或高为 1 格的占比", "compact_mix": "面积至多 2 格的占比",
	}[topic]
}

func reservedDraws(spec *Specialty) int {
	if spec.ReserveDraws != 0 {
		return spec.ReserveDraws
	}
	return 6
}

func reportDraws(spec *Specialty) int {
	if spec.ReportDraws != 0 {
		return spec.ReportDraws
	}
	return 6
}

func (p *specialtyProgress) plan(round int) *specialtyPlan {
	for i := range p.Plans {
		if p.Plans[i].Round == round {
			return &p.Plans[i]
		}
	}
	return nil
}

func (g *Game) beginSpecialtyRound(p *player, spec *Specialty) {
	if p.specialtyState == nil {
		p.specialtyState = &specialtyProgress{Choices: []specialtyChoice{}}
	}
	s := p.specialtyState
	s.ArmedRound, s.ArmedTopic = 0, ""
	if g.round > 3 {
		return
	}
	topics := append([]string{}, specialtyTopics...)
	for i := range topics {
		j := i + random(len(topics)-i)
		topics[i], topics[j] = topics[j], topics[i]
	}
	plan := specialtyPlan{Round: g.round, Topics: topics[:3], Samples: make([][]string, 3)}
	pool := []string{}
	for _, item := range g.items {
		if !isEasterEgg(item) {
			pool = append(pool, item.ID)
		}
	}
	for i := range plan.Samples {
		plan.Samples[i] = []string{}
		if len(pool) > 0 {
			for range 12 {
				plan.Samples[i] = append(plan.Samples[i], pool[random(len(pool))])
			}
		}
	}
	s.Plans = append(s.Plans, plan)
	if spec.Style == "survey" {
		g.specialtyReport(p, spec, &plan, plan.Topics[0], 6)
	}
	if spec.Style == "research" && g.round == 3 && s.Research != nil && s.Research.Committed {
		g.specialtyReport(p, spec, s.plan(1), s.Research.Topic, 12)
		s.Research.Delivered = true
	}
	if g.round == 3 {
		s.Reserved = false
	}
}

func (g *Game) sampleReport(plan *specialtyPlan, topic string, draws int) map[string]any {
	index := slices.Index(plan.Topics, topic)
	ids := plan.Samples[index]
	data := map[string]any{"type": "sample_report", "topic": topic, "draws": draws, "sampling": "independent_with_replacement"}
	if len(ids) == 0 {
		data["band"] = "unavailable"
		return data
	}
	items := map[string]Item{}
	for _, item := range g.items {
		items[item.ID] = item
	}
	count := 0
	for _, id := range ids[:draws] {
		item := items[id]
		rare := item.Quality == "rare" || item.Quality == "epic" || item.Quality == "legendary"
		match := false
		switch topic {
		case "quality_mix":
			match = rare
		case "upper_mix":
			match = item.Quality == "epic" || item.Quality == "legendary"
		case "space_mix":
			match = item.area() >= 4
		case "cross_mix":
			match = rare && item.area() >= 4
		case "slender_mix":
			match = item.BottomRight.X-item.TopLeft.X == 1 || item.BottomRight.Y-item.TopLeft.Y == 1
		case "compact_mix":
			match = item.area() <= 2
		}
		if match {
			count++
		}
	}
	band := "mixed"
	if count*3 < draws {
		band = "low"
	} else if count*3 >= draws*2 {
		band = "high"
	}
	data["band"] = band
	return data
}

func (g *Game) specialtyReport(p *player, spec *Specialty, plan *specialtyPlan, topic string, draws int) {
	g.addSignal(p, spec.Name, g.sampleReport(plan, topic, draws))
}

func (g *Game) specialtyStatus(p *player) *SpecialtyStatus {
	spec := findSpecialty(g.gameConfig.Specialties, p.specialty)
	if spec == nil || spec.Style == "" || p.specialtyState == nil {
		return nil
	}
	s := p.specialtyState
	out := &SpecialtyStatus{Style: spec.Style, Round: g.round, Actions: []string{}, Questions: []SpecialtyQuestion{}, Choices: append([]specialtyChoice{}, s.Choices...), Reserved: s.Reserved, Preparation: len(s.Preparation), BoostArmed: s.ArmedRound == g.round}
	if s.Research != nil {
		copy := *s.Research
		out.Research = &copy
	}
	if g.phase != "bidding" || p.withdrawn || p.bid != nil || g.round > 3 || !time.Now().Before(g.roundDeadline) {
		return out
	}
	for _, c := range s.Choices {
		if c.Round == g.round {
			return out
		}
	}
	switch spec.Style {
	case "quick":
		if g.round == 1 {
			out.Actions = []string{"take", "reserve"}
		} else if g.round == 2 && s.Reserved {
			out.Actions = []string{"take"}
		}
	case "research":
		if g.round == 1 {
			out.Actions = []string{"start"}
		} else if g.round == 2 {
			out.Actions = []string{"take"}
			if s.Research != nil {
				out.Actions = append(out.Actions, "commit")
			}
		}
	case "planner":
		out.Actions = []string{"take"}
		if len(s.Preparation) < 2 {
			out.Actions = append(out.Actions, "bank")
		}
		if len(s.Preparation) > 0 && s.Preparation[0] < g.round && p.cardRound != g.round {
			out.Actions = append(out.Actions, "boost")
		}
	case "adaptive":
		if g.round >= 2 {
			out.Actions = []string{"choose"}
		}
	}
	if len(out.Actions) > 0 && spec.Style != "quick" {
		for _, topic := range s.plan(g.round).Topics {
			out.Questions = append(out.Questions, SpecialtyQuestion{topic, topicName(topic)})
		}
	}
	return out
}

// SpecialtyAction is private, like using a card. It consumes neither the public
// version nor a card allowance; each player may make one choice before bidding.
func (g *Game) SpecialtyAction(token string, round int, action, topic, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.find(token)
	if p == nil {
		return ErrUnauthorized
	}
	if err := ValidateReason(reason); err != nil {
		return err
	}
	status := g.specialtyStatus(p)
	if round != g.round || status == nil || !slices.Contains(status.Actions, action) {
		return errors.New("specialty action unavailable; read your current state before choosing")
	}
	spec := findSpecialty(g.gameConfig.Specialties, p.specialty)
	s := p.specialtyState
	plan := s.plan(round)
	needsTopic := action == "start" || action == "choose" || spec.Style == "planner" && (action == "take" || action == "boost") || spec.Style == "research" && action == "take"
	if needsTopic && !slices.Contains(plan.Topics, topic) {
		return errors.New("choose a topic from self.specialty_state.questions")
	}
	if !needsTopic && topic != "" {
		return errors.New("this specialty action does not accept a topic")
	}
	switch spec.Style {
	case "quick":
		if round == 1 {
			g.specialtyReport(p, spec, plan, plan.Topics[0], 6)
			if action == "take" {
				g.specialtyReport(p, spec, plan, plan.Topics[1], 6)
			} else {
				s.Reserved = true
			}
		} else {
			first := s.plan(1)
			g.specialtyReport(p, spec, first, first.Topics[1], reservedDraws(spec))
			s.Reserved = false
		}
	case "research":
		if action == "commit" {
			s.Research.Committed = true
		} else {
			g.specialtyReport(p, spec, plan, topic, 6)
			if action == "start" {
				s.Research = &ResearchStatus{Topic: topic, DueRound: 3}
			} else if round == 2 {
				s.Research = nil
			}
		}
	case "planner":
		switch action {
		case "take":
			g.specialtyReport(p, spec, plan, topic, reportDraws(spec))
		case "bank":
			s.Preparation = append(s.Preparation, round)
		case "boost":
			s.Preparation = s.Preparation[1:]
			s.ArmedRound, s.ArmedTopic = round, topic
		}
	case "adaptive":
		g.specialtyReport(p, spec, plan, topic, 6)
	}
	s.Choices = append(s.Choices, specialtyChoice{round, action, topic})
	p.revision++
	g.decisions = append(g.decisions, Decision{round, p.id, "specialty", reason})
	return nil
}

func (s gameSnapshot) validateSpecialty(p playerSnapshot) error {
	spec := findSpecialty(s.Config.Specialties, p.Specialty)
	state := p.SpecialtyState
	if spec.Style == "" || s.Phase == "lobby" {
		if state != nil {
			return errors.New("unexpected specialty progress")
		}
		return nil
	}
	lastRound := s.Round
	if p.Withdrawn {
		lastRound = p.WithdrawRound
	}
	if state == nil || len(state.Plans) != min(lastRound, 3) {
		return errors.New("missing specialty progress")
	}
	items := map[string]bool{}
	for _, item := range s.Items {
		items[item.ID] = !isEasterEgg(item)
	}
	for i, plan := range state.Plans {
		if plan.Round != i+1 || plan.Round > min(s.Round, 3) || len(plan.Topics) != 3 || len(plan.Samples) != 3 {
			return errors.New("invalid specialty plan")
		}
		seen := map[string]bool{}
		for j, topic := range plan.Topics {
			if !slices.Contains(specialtyTopics, topic) || seen[topic] || len(plan.Samples[j]) != 12 && len(plan.Samples[j]) != 0 {
				return errors.New("invalid specialty question")
			}
			seen[topic] = true
			if len(plan.Samples[j]) == 0 {
				for _, ordinary := range items {
					if ordinary {
						return errors.New("missing specialty sample")
					}
				}
			}
			for _, id := range plan.Samples[j] {
				if !items[id] {
					return errors.New("invalid specialty sample item")
				}
			}
		}
	}
	last := 0
	credits := []int{}
	reserved := false
	var research *ResearchStatus
	armedRound, armedTopic := 0, ""
	for _, c := range state.Choices {
		if c.Round <= last || c.Round > min(s.Round, 3) || p.WithdrawRound > 0 && c.Round > p.WithdrawRound || state.plan(c.Round) == nil {
			return errors.New("invalid specialty choice round")
		}
		last = c.Round
		needsTopic := false
		valid := false
		switch spec.Style {
		case "quick":
			valid = c.Round == 1 && (c.Action == "take" || c.Action == "reserve") || c.Round == 2 && reserved && c.Action == "take"
			reserved = c.Round == 1 && c.Action == "reserve"
		case "research":
			needsTopic = c.Action != "commit"
			switch c.Action {
			case "start":
				valid = c.Round == 1
				research = &ResearchStatus{Topic: c.Topic, DueRound: 3}
			case "commit":
				valid = c.Round == 2 && research != nil
				if research != nil {
					research.Committed = true
				}
			case "take":
				valid = c.Round == 2
				if c.Round == 2 {
					research = nil
				}
			}
		case "planner":
			switch c.Action {
			case "take":
				valid, needsTopic = true, true
			case "bank":
				valid = len(credits) < 2
				credits = append(credits, c.Round)
			case "boost":
				valid, needsTopic = len(credits) > 0 && credits[0] < c.Round, true
				if valid {
					credits = credits[1:]
					armedRound, armedTopic = c.Round, c.Topic
				}
			}
		case "adaptive":
			valid, needsTopic = c.Round >= 2 && c.Action == "choose", true
		}
		if !valid || needsTopic && !slices.Contains(state.plan(c.Round).Topics, c.Topic) || !needsTopic && c.Topic != "" {
			return errors.New("invalid specialty choice")
		}
	}
	if s.Round >= 3 && state.plan(3) != nil {
		reserved = false
		if research != nil && research.Committed {
			research.Delivered = true
		}
	}
	if armedRound != lastRound || p.CardRound >= armedRound {
		armedRound, armedTopic = 0, ""
	}
	if reserved != state.Reserved || !slices.Equal(credits, state.Preparation) || armedRound != state.ArmedRound || armedTopic != state.ArmedTopic {
		return errors.New("inconsistent specialty resources")
	}
	if (research == nil) != (state.Research == nil) || research != nil && *research != *state.Research {
		return errors.New("inconsistent research commitment")
	}
	// Recompute only from already frozen draws. This validates stored answers
	// without generating new information or replaying economic actions.
	g := &Game{items: s.Items}
	expected := &player{specialty: p.Specialty, signals: []Hint{}}
	for _, plan := range state.Plans {
		g.round = plan.Round
		if spec.Style == "survey" {
			g.specialtyReport(expected, spec, &plan, plan.Topics[0], 6)
		}
		if spec.Style == "research" && plan.Round == 3 && research != nil && research.Committed {
			g.specialtyReport(expected, spec, state.plan(1), research.Topic, 12)
		}
		for _, c := range state.Choices {
			if c.Round != plan.Round {
				continue
			}
			if spec.Style == "quick" {
				if c.Round == 1 {
					g.specialtyReport(expected, spec, &plan, plan.Topics[0], 6)
					if c.Action == "take" {
						g.specialtyReport(expected, spec, &plan, plan.Topics[1], 6)
					}
				} else {
					first := state.plan(1)
					g.specialtyReport(expected, spec, first, first.Topics[1], reservedDraws(spec))
				}
			} else if c.Action == "start" || c.Action == "take" || c.Action == "choose" {
				g.specialtyReport(expected, spec, &plan, c.Topic, reportDraws(spec))
			}
			if c.Action == "boost" {
				for _, h := range p.Hints {
					if h.Round != c.Round {
						continue
					}
					var report map[string]any
					raw, _ := json.Marshal(h.Data["preparation_report"])
					if json.Unmarshal(raw, &report) != nil || !sameReport(report, g.sampleReport(&plan, c.Topic, 12)) {
						return errors.New("invalid preparation report")
					}
					for _, card := range s.Cards.Cards {
						if card.ID == h.Kind && card.Effect.RevealsPrice() {
							return errors.New("preparation used for exact appraisal")
						}
					}
				}
			}
		}
	}
	a, _ := json.Marshal(expected.signals)
	b, _ := json.Marshal(p.Signals)
	if !bytes.Equal(a, b) {
		return errors.New("specialty reports do not match frozen choices and samples")
	}
	return nil
}

func sameReport(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
