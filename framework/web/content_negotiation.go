package web

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// MediaType represents a parsed media type with its quality factor (q).
type MediaType struct {
	Type textProto
	Q    float64
}

type textProto struct {
	mainSub string // e.g. "application/json"
}

// XMLMap is a custom map type that implements xml.Marshaler to allow
// encoding of dynamic maps into XML elements, preventing errors with standard library.
type XMLMap map[string]any

// xmlNameRegex matches characters that are invalid in XML local names
var xmlNameRegex = regexp.MustCompile(`[^a-zA-Z0-9.\-_:]`)

// sanitizeXMLName ensures a string is a valid XML element name
func sanitizeXMLName(name string) string {
	if len(name) == 0 {
		return "entry"
	}
	// Check first character
	runes := []rune(name)
	var sb strings.Builder
	first := runes[0]
	if !unicode.IsLetter(first) && first != '_' && first != ':' {
		sb.WriteRune('_')
	}
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == ':' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	res := sb.String()
	// Replace invalid combinations
	res = xmlNameRegex.ReplaceAllString(res, "_")
	if len(res) == 0 {
		return "entry"
	}
	return res
}

// MarshalXML implements xml.Marshaler interface
func (m XMLMap) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for k, v := range m {
		sanitizedKey := sanitizeXMLName(k)
		elem := xml.StartElement{Name: xml.Name{Local: sanitizedKey}}
		if err := e.EncodeElement(v, elem); err != nil {
			return err
		}
	}
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// XMLResponseWrapper wraps any negotiated XML data structure to ensure
// it has a custom root element name and implements xml.Marshaler.
type XMLResponseWrapper struct {
	XMLName xml.Name
	Value   any
}

// MarshalXML implements custom XML serialization for XMLResponseWrapper
func (w XMLResponseWrapper) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = w.XMLName
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	if xmlMap, ok := w.Value.(XMLMap); ok {
		for k, v := range xmlMap {
			sanitizedKey := sanitizeXMLName(k)
			elem := xml.StartElement{Name: xml.Name{Local: sanitizedKey}}
			if err := e.EncodeElement(v, elem); err != nil {
				return err
			}
		}
		return e.EncodeToken(xml.EndElement{Name: start.Name})
	}

	if slice, ok := w.Value.([]any); ok {
		for _, item := range slice {
			elem := xml.StartElement{Name: xml.Name{Local: "item"}}
			if err := e.EncodeElement(item, elem); err != nil {
				return err
			}
		}
		return e.EncodeToken(xml.EndElement{Name: start.Name})
	}

	// Fallback for primitive values
	charData := fmt.Sprintf("%v", w.Value)
	if err := e.EncodeToken(xml.CharData(charData)); err != nil {
		return err
	}
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// ToXMLValue recursively converts structs, maps and slices to XML-friendly structures
func ToXMLValue(val any) any {
	if val == nil {
		return nil
	}

	v := reflect.ValueOf(val)
	// Handle pointers and interfaces
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Map:
		m := make(XMLMap)
		iter := v.MapRange()
		for iter.Next() {
			key := fmt.Sprintf("%v", iter.Key().Interface())
			m[key] = ToXMLValue(iter.Value().Interface())
		}
		return m

	case reflect.Slice, reflect.Array:
		l := v.Len()
		res := make([]any, l)
		for i := 0; i < l; i++ {
			res[i] = ToXMLValue(v.Index(i).Interface())
		}
		return res

	case reflect.Struct:
		m := make(XMLMap)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" { // Skip unexported fields
				continue
			}

			// Determine XML element name
			name := field.Name
			if xmlTag := field.Tag.Get("xml"); xmlTag != "" {
				if xmlTag == "-" {
					continue
				}
				parts := strings.Split(xmlTag, ",")
				name = parts[0]
			} else if jsonTag := field.Tag.Get("json"); jsonTag != "" {
				if jsonTag == "-" {
					continue
				}
				parts := strings.Split(jsonTag, ",")
				name = parts[0]
			}

			// Skip XMLName field to avoid duplication
			if field.Type == reflect.TypeOf(xml.Name{}) {
				continue
			}

			m[name] = ToXMLValue(v.Field(i).Interface())
		}
		return m

	default:
		return v.Interface()
	}
}

// ResolveNegotiatedFormat determines the best format (json, xml, yaml)
// based on Accept header quality parameters or a "format" query parameter.
func ResolveNegotiatedFormat(r *http.Request) string {
	// 1. Favor query parameter (e.g. ?format=xml) if present (Spring Boot style)
	if fmtParam := r.URL.Query().Get("format"); fmtParam != "" {
		switch strings.ToLower(fmtParam) {
		case "xml":
			return "xml"
		case "yaml", "yml":
			return "yaml"
		case "json":
			return "json"
		}
	}

	// 2. Fallback to Accept header parsing
	acceptHeader := r.Header.Get("Accept")
	if acceptHeader == "" || acceptHeader == "*/*" {
		return "json"
	}

	parts := strings.Split(acceptHeader, ",")
	mediaTypes := make([]MediaType, 0, len(parts))

	for _, part := range parts {
		subparts := strings.Split(part, ";")
		mediaTypeStr := strings.TrimSpace(subparts[0])
		qVal := 1.0

		for _, sub := range subparts[1:] {
			sub = strings.TrimSpace(sub)
			if strings.HasPrefix(sub, "q=") {
				if q, err := strconv.ParseFloat(sub[2:], 64); err == nil {
					qVal = q
				}
			}
		}
		mediaTypes = append(mediaTypes, MediaType{
			Type: textProto{mainSub: strings.ToLower(mediaTypeStr)},
			Q:    qVal,
		})
	}

	// Sort by Quality factor descending
	sort.Slice(mediaTypes, func(i, j int) bool {
		return mediaTypes[i].Q > mediaTypes[j].Q
	})

	for _, mt := range mediaTypes {
		mtStr := mt.Type.mainSub
		if strings.Contains(mtStr, "application/json") || mtStr == "*/*" {
			return "json"
		}
		if strings.Contains(mtStr, "application/xml") || strings.Contains(mtStr, "text/xml") {
			return "xml"
		}
		if strings.Contains(mtStr, "application/x-yaml") || strings.Contains(mtStr, "text/yaml") || strings.Contains(mtStr, "application/yaml") {
			return "yaml"
		}
	}

	return "json" // Default fallback
}

// WriteResponse negotiates content type and serializes data dynamically
func WriteResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
	format := ResolveNegotiatedFormat(r)

	switch format {
	case "xml":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(xml.Header)); err != nil {
			return
		}

		// Convert data recursively to XMLMap representation for safe serializing
		xmlVal := ToXMLValue(data)
		wrapped := XMLResponseWrapper{
			XMLName: xml.Name{Local: "response"},
			Value:   xmlVal,
		}
		_ = xml.NewEncoder(w).Encode(wrapped)

	case "yaml":
		w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
		w.WriteHeader(status)
		_ = yaml.NewEncoder(w).Encode(data)

	default: // json
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(data)
	}
}
