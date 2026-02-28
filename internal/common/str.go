package common

import "github.com/google/uuid"

type StringSet struct {
	seen map[string]bool
}

type StringSetOption func(*StringSet)

func WithValues(values []string) StringSetOption {
	return func(s *StringSet) {
		s.seen = make(map[string]bool, len(values))
		for _, v := range values {
			s.Insert(v)
		}
	}
}

func NewStringSet(opts ...StringSetOption) *StringSet {
	s := &StringSet{}
	for _, opt := range opts {
		opt(s)
	}
	if s.seen == nil {
		s.seen = make(map[string]bool)
	}
	return s
}

// return array in random order.
func (s *StringSet) ToSlice() []string {
	var ret []string
	for key := range s.seen {
		ret = append(ret, key)
	}
	return ret
}

func (s *StringSet) Insert(value string) {
	if _, ok := s.seen[value]; ok {
		return
	}
	s.seen[value] = true
}

// TODO(3): move it to a proper place.
func ParseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(s)
}
