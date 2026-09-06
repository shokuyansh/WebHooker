package validator

import "net/url"

type Validator struct {
	Errors map[string]string
}

func New() *Validator {
	return &Validator{
		Errors: make(map[string]string),
	}
}

func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) addError(key, value string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = value
	}
}

func (v *Validator) Check(ok bool, key, value string) {
	if !ok {
		v.addError(key, value)
	}
}

func ValidURL(callback_url string) bool {
	_, err := url.ParseRequestURI(callback_url)
	if err != nil {
		return false
	}
	return true
}
