package lifecycle

import (
	"encoding/xml"
	"errors"
	"fmt"
)

type Configuration struct {
	XMLName xml.Name `xml:"LifecycleConfiguration"`
	Rules   []Rule   `xml:"Rule"`
	UNKNOWN []Any    `xml:",any"`
}

func (c *Configuration) Visit(v Visitor) {
	v.VisitConfiguration(c)
	for _, r := range c.Rules {
		r.Visit(v)
	}
}

type Rule struct {
	ID          *string
	Filter      *Filter // Can there be multiple filters
	Status      string
	Transitions []Transition `xml:"Transition"`
	Expiration  *Expiration  // Assume that there can be only 1
	UNKNOWN     []Any        `xml:",any"`
}

func (r *Rule) Visit(v Visitor) {
	v.VisitRule(r)
	if r.Filter != nil {
		r.Filter.Visit(v)
	}
	for _, t := range r.Transitions {
		t.Visit(v)
	}
	if r.Expiration != nil {
		r.Expiration.Visit(v)
	}
}

type Filter struct {
	Prefix  *string `xml:"Prefix"`
	Tag     *Tag
	And     *And
	UNKNOWN []Any `xml:",any"`
}

func (f *Filter) Visit(v Visitor) {
	v.VisitFilter(f)
	if f.Tag != nil {
		f.Tag.Visit(v)
	}
	if f.And != nil {
		f.And.Visit(v)
	}
}

type Tag struct {
	Key     string
	Value   string
	UNKNOWN []Any `xml:",any"`
}

func (t *Tag) Visit(v Visitor) {
	v.VisitTag(t)
}

type And struct {
	Prefix  *string `xml:"Prefix"`
	Tag     *Tag
	UNKNOWN []Any `xml:",any"`
}

func (a *And) Visit(v Visitor) {
	v.VisitAnd(a)
	if a.Tag != nil {
		a.Tag.Visit(v)
	}
}

type Transition struct {
	Days         *int
	Date         *string
	StorageClass string
	UNKNOWN      []Any `xml:",any"`
}

func (t *Transition) Visit(v Visitor) {
	v.VisitTransition(t)
}

type Expiration struct {
	Days    *int
	Date    *string
	UNKNOWN []Any `xml:",any"`
}

func (e *Expiration) Visit(v Visitor) {
	v.VisitExpiration(e)
}

type Any struct {
	XMLName xml.Name
	XML     string `xml:",innerxml"`
}

type Visitor interface {
	VisitConfiguration(c *Configuration)
	VisitRule(r *Rule)
	VisitFilter(f *Filter)
	VisitTag(t *Tag)
	VisitAnd(a *And)
	VisitTransition(t *Transition)
	VisitExpiration(e *Expiration)
}

// A Visitor that does nothing, useful for embedding.
type NullVisitor struct{}

func (v *NullVisitor) VisitConfiguration(c *Configuration) {}
func (v *NullVisitor) VisitRule(r *Rule)                   {}
func (v *NullVisitor) VisitFilter(f *Filter)               {}
func (v *NullVisitor) VisitTag(t *Tag)                     {}
func (v *NullVisitor) VisitAnd(a *And)                     {}
func (v *NullVisitor) VisitTransition(t *Transition)       {}
func (v *NullVisitor) VisitExpiration(e *Expiration)       {}

type uncleanVisitor struct {
	unc bool
}

func (c *Configuration) IsUnclean() bool {
	v := &uncleanVisitor{}
	c.Visit(v)
	return v.unc
}

func (v *uncleanVisitor) VisitConfiguration(c *Configuration) {
	v.unc = v.unc || len(c.UNKNOWN) > 0
}

func (v *uncleanVisitor) VisitRule(r *Rule) {
	v.unc = v.unc || len(r.UNKNOWN) > 0
}

func (v *uncleanVisitor) VisitFilter(f *Filter) {
	v.unc = v.unc || len(f.UNKNOWN) > 0
}

func (v *uncleanVisitor) VisitTag(t *Tag) {
	v.unc = v.unc || len(t.UNKNOWN) > 0
}

func (v *uncleanVisitor) VisitAnd(a *And) {
	v.unc = v.unc || len(a.UNKNOWN) > 0
}

func (v *uncleanVisitor) VisitTransition(t *Transition) {
	v.unc = v.unc || len(t.UNKNOWN) > 0
}

func (v *uncleanVisitor) VisitExpiration(e *Expiration) {
	v.unc = v.unc || len(e.UNKNOWN) > 0
}

func (c *Configuration) CheckValues() []error {
	v := &valCh{}
	c.Visit(v)
	return v.errors
}

type valCh struct {
	NullVisitor // We don't want to implement the Visit methods for all types.
	errors      []error
}

func (v *valCh) add(s string) {
	v.errors = append(v.errors, errors.New(s))
}

func (v *valCh) addf(format string, args ...interface{}) {
	v.add(fmt.Sprintf(format, args...))
}

func (v *valCh) VisitRule(r *Rule) {
	if !(r.Status == "Enabled" || r.Status == "Disabled") {
		v.addf("Rule Status must be 'Enabled' or 'Disabled', got: %q", r.Status)
	}
}

func (v *valCh) VisitTransition(t *Transition) {
	if t.Days != nil && t.Date != nil {
		v.add("Transtion cannot have both a Date and Day")
	}
	if t.Days != nil && *t.Days < 0 {
		v.addf("Days cannot be negative, got: %d", *t.Days)
	}
	if !(t.StorageClass == "STANDARD_IA" || t.StorageClass == "GLACIER") {
		v.addf("StorageClass ,must be on of ('STANDARD_IA', 'GLACIER'), got: %q", t.StorageClass)
	}

}
