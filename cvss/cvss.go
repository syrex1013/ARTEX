// Package cvss parses and scores CVSS v3.1 base vectors (https://www.first.org/cvss/v3.1/specification-document).
// Only the base metric group is supported: temporal and environmental scores are not computed.
package cvss

import (
	"fmt"
	"math"
	"strings"
)

// Result is the outcome of scoring a vector.
type Result struct {
	Vector         string  `json:"vector"`
	Score          float64 `json:"score"`
	Severity       string  `json:"severity"` // none | low | medium | high | critical
	Impact         float64 `json:"impact"`
	Exploitability float64 `json:"exploitability"`
}

var (
	weightAV  = map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}
	weightAC  = map[string]float64{"L": 0.77, "H": 0.44}
	weightUI  = map[string]float64{"N": 0.85, "R": 0.62}
	weightCIA = map[string]float64{"H": 0.56, "L": 0.22, "N": 0}
)

// roundUp is the spec's Roundup: the smallest number with one decimal that is >= x, immune to float noise.
func roundUp(x float64) float64 {
	i := int(math.Round(x * 100000))
	if i%10000 == 0 {
		return float64(i) / 100000
	}
	return float64(i/10000+1) / 10
}

// Parse validates a "CVSS:3.1/AV:N/AC:L/..." vector (3.0 prefix accepted, same base formula) and returns its metrics.
func Parse(vector string) (map[string]string, error) {
	v := strings.TrimSpace(vector)
	parts := strings.Split(v, "/")
	if len(parts) < 9 || (parts[0] != "CVSS:3.1" && parts[0] != "CVSS:3.0") {
		return nil, fmt.Errorf("not a CVSS 3.x vector (want CVSS:3.1/AV:./AC:./PR:./UI:./S:./C:./I:./A:.)")
	}
	m := map[string]string{}
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 || kv[0] == "" || kv[1] == "" {
			return nil, fmt.Errorf("malformed metric %q", p)
		}
		if _, dup := m[kv[0]]; dup {
			return nil, fmt.Errorf("metric %s given twice", kv[0])
		}
		m[kv[0]] = kv[1]
	}
	allowed := map[string]string{"AV": "NALP", "AC": "LH", "PR": "NLH", "UI": "NR", "S": "UC", "C": "HLN", "I": "HLN", "A": "HLN"}
	for k, vals := range allowed {
		got, ok := m[k]
		if !ok {
			return nil, fmt.Errorf("base metric %s is missing", k)
		}
		if len(got) != 1 || !strings.Contains(vals, got) {
			return nil, fmt.Errorf("metric %s has an invalid value %q (allowed: %s)", k, got, strings.Join(strings.Split(vals, ""), ", "))
		}
	}
	return m, nil
}

// Score computes the base score of a vector.
func Score(vector string) (*Result, error) {
	m, err := Parse(vector)
	if err != nil {
		return nil, err
	}
	changed := m["S"] == "C"
	pr := map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}[m["PR"]]
	if changed {
		pr = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}[m["PR"]]
	}
	iss := 1 - (1-weightCIA[m["C"]])*(1-weightCIA[m["I"]])*(1-weightCIA[m["A"]])
	var impact float64
	if changed {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}
	expl := 8.22 * weightAV[m["AV"]] * weightAC[m["AC"]] * pr * weightUI[m["UI"]]
	var base float64
	switch {
	case impact <= 0:
		base = 0
	case changed:
		base = roundUp(math.Min(1.08*(impact+expl), 10))
	default:
		base = roundUp(math.Min(impact+expl, 10))
	}
	return &Result{
		Vector: "CVSS:3.1/" + strings.Join(orderedMetrics(m), "/"), Score: base, Severity: Severity(base),
		Impact: math.Round(impact*10) / 10, Exploitability: math.Round(expl*10) / 10,
	}, nil
}

func orderedMetrics(m map[string]string) []string {
	var out []string
	for _, k := range []string{"AV", "AC", "PR", "UI", "S", "C", "I", "A"} {
		out = append(out, k+":"+m[k])
	}
	return out
}

// Severity maps a score to the qualitative rating of the specification.
func Severity(score float64) string {
	switch {
	case score <= 0:
		return "none"
	case score < 4:
		return "low"
	case score < 7:
		return "medium"
	case score < 9:
		return "high"
	default:
		return "critical"
	}
}
