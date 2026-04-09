package render

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// BuildFuncMap creates the template FuncMap with all helper functions.
func BuildFuncMap() template.FuncMap {
	return template.FuncMap{
		// String functions
		"slug":      entity.ToSlug,
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     strings.Title,
		"join":      strings.Join,
		"split":     strings.Split,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"trimSpace": strings.TrimSpace,
		"urlencode": url.QueryEscape,

		// Number functions
		"formatNumber": formatNumber,
		"add":          func(a, b int) int { return a + b },
		"sub":          func(a, b int) int { return a - b },
		"mul":          func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"mod": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a % b
		},
		"addf": func(a, b float64) float64 { return a + b },
		"mulf": func(a, b float64) float64 { return a * b },

		// Duration functions
		"durationMinutes": durationMinutes,
		"totalTime":       totalTime,
		"formatDuration":  formatDuration,

		// Collection functions
		"first":   first,
		"last":    last,
		"seq":     seq,
		"dict":    dict,
		"slice":   sliceHelper,
		"len":     length,
		"sort":    sortStrings,
		"reverse": reverseStrings,
		"min":     minInt,
		"max":     maxInt,

		// Entity functions
		"field":          fieldAccess,
		"section":        sectionAccess,
		"getStringSlice": getStringSlice,
		"hasField":       hasField,
		"getInt":         getInt,
		"getFloat":       getFloat,

		// JSON/HTML functions
		"jsonMarshal": jsonMarshal,
		"safeHTML":    func(s string) template.HTML { return template.HTML(s) },
		"safeJS":      func(s string) template.JS { return template.JS(s) },
		"safeCSS":     func(s string) template.CSS { return template.CSS(s) },
		"safeURL":     func(s string) template.URL { return template.URL(s) },
		"safeAttr":    func(s string) template.HTMLAttr { return template.HTMLAttr(s) },

		// Ingredient parsing
		"parseIngredientQty":  parseIngredientQty,
		"parseIngredientUnit": parseIngredientUnit,
		"parseIngredientDesc": parseIngredientDesc,
		"fractionDisplay":     fractionDisplay,
		"scaleQty":            scaleQty,

		// Conditionals
		"default": defaultVal,
		"ternary": ternary,
		"hasKey":  hasKey,

		// Comparison
		"eq": func(a, b interface{}) bool { return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) },
		"ne": func(a, b interface{}) bool { return fmt.Sprintf("%v", a) != fmt.Sprintf("%v", b) },
		"lt": func(a, b int) bool { return a < b },
		"le": func(a, b int) bool { return a <= b },
		"gt": func(a, b int) bool { return a > b },
		"ge": func(a, b int) bool { return a >= b },

		// Misc
		"toJSON":   toJSON,
		"noescape": func(s string) template.HTML { return template.HTML(s) },
	}
}

// formatNumber adds thousands separators to a number.
func formatNumber(n interface{}) string {
	var num int64
	switch v := n.(type) {
	case int:
		num = int64(v)
	case int64:
		num = v
	case float64:
		num = int64(v)
	default:
		return fmt.Sprintf("%v", n)
	}

	s := strconv.FormatInt(num, 10)
	if len(s) <= 3 {
		return s
	}

	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

var isoDuration = regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)

// durationMinutes converts an ISO 8601 duration to minutes, truncating any
// remaining seconds.
func durationMinutes(d string) int {
	matches := isoDuration.FindStringSubmatch(d)
	if matches == nil {
		return 0
	}
	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	seconds, _ := strconv.Atoi(matches[3])
	return hours*60 + minutes + seconds/60
}

// totalTime adds two ISO 8601 durations.
func totalTime(d1, d2 string) string {
	total := durationMinutes(d1) + durationMinutes(d2)
	hours := total / 60
	minutes := total % 60
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("PT%dH%dM", hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("PT%dH", hours)
	}
	return fmt.Sprintf("PT%dM", minutes)
}

// formatDuration converts an ISO 8601 duration to human-readable form.
func formatDuration(d string) string {
	minutes := durationMinutes(d)
	if minutes == 0 {
		return d
	}
	hours := minutes / 60
	mins := minutes % 60
	if hours > 0 && mins > 0 {
		return fmt.Sprintf("%d hr %d min", hours, mins)
	}
	if hours > 0 {
		if hours == 1 {
			return "1 hr"
		}
		return fmt.Sprintf("%d hrs", hours)
	}
	return fmt.Sprintf("%d min", mins)
}

func first(list interface{}) interface{} {
	switch v := list.(type) {
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	case []interface{}:
		if len(v) > 0 {
			return v[0]
		}
	case []*entity.Entity:
		if len(v) > 0 {
			return v[0]
		}
	}
	return nil
}

func last(list interface{}) interface{} {
	switch v := list.(type) {
	case []string:
		if len(v) > 0 {
			return v[len(v)-1]
		}
	case []interface{}:
		if len(v) > 0 {
			return v[len(v)-1]
		}
	}
	return nil
}

// seq generates a sequence of integers from 1 to n.
func seq(n int) []int {
	result := make([]int, n)
	for i := range result {
		result[i] = i + 1
	}
	return result
}

// dict creates a map from alternating key/value pairs.
func dict(values ...interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for i := 0; i+1 < len(values); i += 2 {
		key := fmt.Sprintf("%v", values[i])
		result[key] = values[i+1]
	}
	return result
}

func sliceHelper(items interface{}, start, end int) interface{} {
	switch v := items.(type) {
	case []string:
		if start < 0 {
			start = 0
		}
		if end > len(v) {
			end = len(v)
		}
		return v[start:end]
	case []*entity.Entity:
		if start < 0 {
			start = 0
		}
		if end > len(v) {
			end = len(v)
		}
		return v[start:end]
	case []interface{}:
		if start < 0 {
			start = 0
		}
		if end > len(v) {
			end = len(v)
		}
		return v[start:end]
	}
	return items
}

func length(v interface{}) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.String:
		return rv.Len()
	}
	return 0
}

func sortStrings(s []string) []string {
	result := make([]string, len(s))
	copy(result, s)
	return result
}

func reverseStrings(s []string) []string {
	result := make([]string, len(s))
	for i, v := range s {
		result[len(s)-1-i] = v
	}
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func fieldAccess(e *entity.Entity, key string) interface{} {
	if e == nil {
		return nil
	}
	return e.Fields[key]
}

func sectionAccess(e *entity.Entity, key string) interface{} {
	if e == nil {
		return nil
	}
	return e.Sections[key]
}

func getStringSlice(e *entity.Entity, key string) []string {
	if e == nil {
		return nil
	}
	return e.GetStringSlice(key)
}

func hasField(e *entity.Entity, key string) bool {
	if e == nil {
		return false
	}
	return e.HasField(key)
}

func getInt(e *entity.Entity, key string) int {
	if e == nil {
		return 0
	}
	return e.GetInt(key)
}

func getFloat(e *entity.Entity, key string) float64 {
	if e == nil {
		return 0
	}
	return e.GetFloat(key)
}

func jsonMarshal(v interface{}) template.JS {
	data, err := json.Marshal(v)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(data)
}

func toJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func defaultVal(def, val interface{}) interface{} {
	if val == nil {
		return def
	}
	if s, ok := val.(string); ok && s == "" {
		return def
	}
	return val
}

func ternary(cond bool, trueVal, falseVal interface{}) interface{} {
	if cond {
		return trueVal
	}
	return falseVal
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}

// Ingredient parsing functions

// Unicode fraction map
var unicodeFractions = map[rune]float64{
	'\u00BD': 0.5,
	'\u2153': 1.0 / 3.0,
	'\u2154': 2.0 / 3.0,
	'\u00BC': 0.25,
	'\u00BE': 0.75,
	'\u215B': 0.125,
	'\u215C': 0.375,
	'\u215D': 0.625,
	'\u215E': 0.875,
	'\u2155': 0.2,
	'\u2156': 0.4,
	'\u2157': 0.6,
	'\u2158': 0.8,
	'\u2159': 1.0 / 6.0,
	'\u215A': 5.0 / 6.0,
}

var unitAliases = map[string]string{
	"cups": "cup", "c": "cup",
	"tablespoons": "tablespoon", "tablespoon": "tablespoon", "tbsp": "tablespoon", "tbsps": "tablespoon", "tbs": "tablespoon",
	"teaspoons": "teaspoon", "teaspoon": "teaspoon", "tsp": "teaspoon", "tsps": "teaspoon",
	"pounds": "pound", "pound": "pound", "lbs": "pound", "lb": "pound",
	"ounces": "ounce", "ounce": "ounce", "oz": "ounce",
	"grams": "gram", "gram": "gram", "g": "gram",
	"kilograms": "kilogram", "kilogram": "kilogram", "kg": "kilogram", "kgs": "kilogram",
	"milliliters": "ml", "milliliter": "ml", "ml": "ml", "mls": "ml",
	"liters": "liter", "liter": "liter", "l": "liter",
	"pints": "pint", "pint": "pint", "pt": "pint",
	"quarts": "quart", "quart": "quart", "qt": "quart", "qts": "quart",
	"gallons": "gallon", "gallon": "gallon", "gal": "gallon",
	"bunches": "bunch", "bunch": "bunch",
	"cloves": "clove", "clove": "clove",
	"heads": "head", "head": "head",
	"cans": "can", "can": "can",
	"packages": "package", "package": "package", "pkg": "package",
	"slices": "slice", "slice": "slice",
	"pieces": "piece", "piece": "piece",
	"sticks": "stick", "stick": "stick",
	"pinches": "pinch", "pinch": "pinch",
	"dashes": "dash", "dash": "dash",
	"sprigs": "sprig", "sprig": "sprig",
	"stalks": "stalk", "stalk": "stalk",
}

var qtyRegex = regexp.MustCompile(`^(\d+)\s+(\d+)/(\d+)`)
var fractionRegex = regexp.MustCompile(`^(\d+)/(\d+)`)
var numberRegex = regexp.MustCompile(`^(\d+(?:\.\d+)?)`)

// parseQuantity extracts a numeric quantity from the beginning of a string.
// Returns the quantity and the remaining string.
func parseQuantity(s string) (float64, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ""
	}

	// Check for unicode fractions at the beginning
	for _, r := range s {
		if v, ok := unicodeFractions[r]; ok {
			rest := strings.TrimSpace(s[len(string(r)):])
			return v, rest
		}
		break
	}

	// Check for mixed number with unicode fraction: "1 ½"
	if m := numberRegex.FindString(s); m != "" {
		rest := strings.TrimSpace(s[len(m):])
		if len(rest) > 0 {
			for _, r := range rest {
				if v, ok := unicodeFractions[r]; ok {
					whole, _ := strconv.ParseFloat(m, 64)
					newRest := strings.TrimSpace(rest[len(string(r)):])
					return whole + v, newRest
				}
				break
			}
		}
	}

	// Check for mixed number: "1 1/2"
	if matches := qtyRegex.FindStringSubmatch(s); matches != nil {
		whole, _ := strconv.ParseFloat(matches[1], 64)
		num, _ := strconv.ParseFloat(matches[2], 64)
		den, _ := strconv.ParseFloat(matches[3], 64)
		if den != 0 {
			rest := strings.TrimSpace(s[len(matches[0]):])
			return whole + num/den, rest
		}
	}

	// Check for simple fraction: "1/2"
	if matches := fractionRegex.FindStringSubmatch(s); matches != nil {
		num, _ := strconv.ParseFloat(matches[1], 64)
		den, _ := strconv.ParseFloat(matches[2], 64)
		if den != 0 {
			rest := strings.TrimSpace(s[len(matches[0]):])
			return num / den, rest
		}
	}

	// Check for decimal or integer
	if m := numberRegex.FindString(s); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		rest := strings.TrimSpace(s[len(m):])
		return v, rest
	}

	return 0, s
}

// parseUnit extracts and normalizes a unit from the beginning of a string.
func parseUnit(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}

	// Try to match a unit word
	words := strings.Fields(s)
	if len(words) == 0 {
		return "", s
	}

	// Check with parenthetical: "(14 ounce)" prefix
	if strings.HasPrefix(s, "(") {
		endParen := strings.Index(s, ")")
		if endParen > 0 {
			// Keep the parenthetical as part of the description
			return "", s
		}
	}

	word := strings.ToLower(strings.TrimRight(words[0], ".,;"))
	if canonical, ok := unitAliases[word]; ok {
		rest := strings.TrimSpace(s[len(words[0]):])
		return canonical, rest
	}

	return "", s
}

// parseIngredientQty returns the numeric quantity from an ingredient line.
func parseIngredientQty(line string) float64 {
	qty, _ := parseQuantity(line)
	return qty
}

// parseIngredientUnit returns the canonical unit from an ingredient line.
func parseIngredientUnit(line string) string {
	_, rest := parseQuantity(line)
	unit, _ := parseUnit(rest)
	return unit
}

// parseIngredientDesc returns the description (everything after qty + unit).
func parseIngredientDesc(line string) string {
	_, rest := parseQuantity(line)
	_, desc := parseUnit(rest)
	return desc
}

// fractionDisplay converts a decimal to a display string with fraction symbols.
func fractionDisplay(f float64) string {
	if f == 0 {
		return "0"
	}

	whole := int(f)
	frac := f - float64(whole)

	// Snap to common fractions
	fracStr := ""
	if math.Abs(frac) < 0.05 {
		fracStr = ""
	} else if math.Abs(frac-0.125) < 0.05 {
		fracStr = "\u215B"
	} else if math.Abs(frac-0.2) < 0.05 {
		fracStr = "\u2155"
	} else if math.Abs(frac-0.25) < 0.05 {
		fracStr = "\u00BC"
	} else if math.Abs(frac-1.0/3.0) < 0.05 {
		fracStr = "\u2153"
	} else if math.Abs(frac-0.375) < 0.05 {
		fracStr = "\u215C"
	} else if math.Abs(frac-0.5) < 0.05 {
		fracStr = "\u00BD"
	} else if math.Abs(frac-0.625) < 0.05 {
		fracStr = "\u215D"
	} else if math.Abs(frac-2.0/3.0) < 0.05 {
		fracStr = "\u2154"
	} else if math.Abs(frac-0.75) < 0.05 {
		fracStr = "\u00BE"
	} else if math.Abs(frac-0.875) < 0.05 {
		fracStr = "\u215E"
	} else {
		// No matching fraction, show decimal
		if whole > 0 {
			return fmt.Sprintf("%.1f", f)
		}
		return fmt.Sprintf("%.2f", f)
	}

	if whole > 0 && fracStr != "" {
		return fmt.Sprintf("%d %s", whole, fracStr)
	}
	if whole > 0 {
		return fmt.Sprintf("%d", whole)
	}
	if fracStr != "" {
		return fracStr
	}
	return fmt.Sprintf("%.1f", f)
}

// scaleQty scales a quantity by ratio and formats with fractions.
func scaleQty(baseQty float64, baseServings, newServings int) string {
	if baseServings == 0 {
		return fractionDisplay(baseQty)
	}
	scaled := baseQty * float64(newServings) / float64(baseServings)
	return fractionDisplay(scaled)
}
