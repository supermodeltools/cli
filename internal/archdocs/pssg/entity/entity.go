package entity

// Entity is a generic content item with map-based fields and parsed body sections.
type Entity struct {
	Slug       string
	SourceFile string
	Fields     map[string]interface{}
	Sections   map[string]interface{} // section name -> content ([]string for lists, []FAQ for faqs, string for markdown)
	Body       string                 // raw markdown body (minus frontmatter)
}

// FAQ represents a question-answer pair extracted from a body section.
type FAQ struct {
	Question string
	Answer   string
}

// GetString returns a string field value, or empty string if not found/not a string.
func (e *Entity) GetString(key string) string {
	v, ok := e.Fields[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// GetStringSlice returns a []string field value, or nil if not found/wrong type.
func (e *Entity) GetStringSlice(key string) []string {
	v, ok := e.Fields[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// GetInt returns an int field value, or 0 if not found/wrong type.
func (e *Entity) GetInt(key string) int {
	v, ok := e.Fields[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}

// GetFloat returns a float64 field value, or 0 if not found/wrong type.
func (e *Entity) GetFloat(key string) float64 {
	v, ok := e.Fields[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	}
	return 0
}

// GetBool returns a bool field value, or false if not found/wrong type.
func (e *Entity) GetBool(key string) bool {
	v, ok := e.Fields[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// GetIngredients returns the ingredients section as []string.
func (e *Entity) GetIngredients() []string {
	v, ok := e.Sections["ingredients"]
	if !ok {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	return nil
}

// GetInstructions returns the instructions section as []string.
func (e *Entity) GetInstructions() []string {
	v, ok := e.Sections["instructions"]
	if !ok {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	return nil
}

// GetFAQs returns the FAQs section as []FAQ.
func (e *Entity) GetFAQs() []FAQ {
	v, ok := e.Sections["faqs"]
	if !ok {
		return nil
	}
	if f, ok := v.([]FAQ); ok {
		return f
	}
	return nil
}

// HasField checks if a field exists and is non-empty.
func (e *Entity) HasField(key string) bool {
	v, ok := e.Fields[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case string:
		return val != ""
	case []interface{}:
		return len(val) > 0
	case []string:
		return len(val) > 0
	case nil:
		return false
	default:
		return true
	}
}
