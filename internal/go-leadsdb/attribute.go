package leadsdb

// AttributeType defines the type of a dynamic attribute.
type AttributeType string

const (
	// AttrText represents a text attribute type.
	AttrText AttributeType = "text"
	// AttrNumber represents a number attribute type.
	AttrNumber AttributeType = "number"
	// AttrBool represents a boolean attribute type.
	AttrBool AttributeType = "bool"
	// AttrList represents a list attribute type.
	AttrList AttributeType = "list"
	// AttrObject represents an object attribute type.
	AttrObject AttributeType = "object"
)

// Attribute represents a dynamic key-value attribute on a lead.
type Attribute struct {
	// Name is the name of the attribute.
	Name string `json:"name"`
	// Type is the type of the attribute.
	Type AttributeType `json:"type"`
	// Value is the value of the attribute.
	Value any `json:"value"`
}

// TextAttr creates a text attribute.
func TextAttr(name, value string) Attribute {
	return Attribute{
		Name:  name,
		Type:  AttrText,
		Value: value,
	}
}

// NumberAttr creates a number attribute.
func NumberAttr(name string, value float64) Attribute {
	return Attribute{
		Name:  name,
		Type:  AttrNumber,
		Value: value,
	}
}

// BoolAttr creates a boolean attribute.
func BoolAttr(name string, value bool) Attribute {
	return Attribute{
		Name:  name,
		Type:  AttrBool,
		Value: value,
	}
}

// ListAttr creates a list attribute.
func ListAttr(name string, value []string) Attribute {
	return Attribute{
		Name:  name,
		Type:  AttrList,
		Value: value,
	}
}

// ObjectAttr creates an object attribute.
func ObjectAttr(name string, value map[string]any) Attribute {
	return Attribute{
		Name:  name,
		Type:  AttrObject,
		Value: value,
	}
}
