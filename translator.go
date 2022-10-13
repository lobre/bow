package bow

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goodsign/monday"
)

const (
	defaultLocale = "en_US"
	placeholder   = "%"
)

type index map[string]string

// Translator allows to translate a message from english to a predefined
// set of locales parsed from csv files. Il also deals with date and time formats.
type Translator struct {
	locales map[string]bool
	dict    map[string]index
	regDict map[string]index // used for translations with placeholders
}

// NewTranslator creates a translator.
func NewTranslator() *Translator {
	return &Translator{
		locales: make(map[string]bool),
		dict:    make(map[string]index),
		regDict: make(map[string]index),
	}
}

// Parse parses all the csv files in the translations folder and
// build dictionnary maps that will serve as databases for translations.
// The name of csv file should be a string representing a locale (e.g. en_US).
// When % is used in a csv translation, it will serve as a placeholder
// and its value won’t be altered during the translation.
func (tr *Translator) Parse(fsys fs.FS) error {
	matches, err := fs.Glob(fsys, "translations/*.csv")
	if err != nil {
		return err
	}

	for _, path := range matches {
		base := filepath.Base(path)
		locale := strings.TrimSuffix(base, filepath.Ext(base))

		re := regexp.MustCompile("^[a-z]{2}_[A-Z]{2}$")
		if !re.MatchString(locale) {
			return fmt.Errorf("locale %s is not valid", locale)
		}

		tr.locales[locale] = true
		tr.dict[locale], tr.regDict[locale], err = parseIndex(fsys, path)
		if err != nil {
			return err
		}
	}

	return nil
}

// parseIndex parses a csv file that contains key values entries into index maps.
// It returns one regular index map for static translations, and a regex index map
// which contains regex patterns and replacements for translations with placeholders.
func parseIndex(fsys fs.FS, path string) (idx index, regIdx index, err error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, nil, err
	}

	idx = make(map[string]string)
	regIdx = make(map[string]string)

	r := csv.NewReader(f)
	for {
		line, err := r.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, nil, err
		}
		if len(line) != 2 {
			return nil, nil, errors.New("error reading csv file")
		}

		pat := fmt.Sprintf("(^|[^%s])%s([^%s]|$)", placeholder, placeholder, placeholder)
		re := regexp.MustCompile(pat)

		// no placeholder found
		if !re.MatchString(line[0]) {
			idx[line[0]] = line[1]
			continue
		}

		key := re.ReplaceAllString(line[0], `${1}(.+)${2}`)

		var i = 0
		val := re.ReplaceAllStringFunc(line[1], func(s string) string {
			i++
			return strings.ReplaceAll(s, placeholder, fmt.Sprintf("${%d}", i))
		})

		regIdx[key] = val
	}

	return idx, regIdx, nil
}

// Translate translates a message into the language of the corresponding locale.
// If the locale or the message is not found, it will be returned untranslated.
func (tr *Translator) Translate(msg string, locale string) string {
	if locale == defaultLocale {
		return msg
	}

	if _, ok := tr.dict[locale]; !ok {
		return msg
	}

	out, ok := tr.dict[locale][msg]
	if ok {
		return out
	}

	for k, v := range tr.regDict[locale] {
		re := regexp.MustCompile(k)
		if !re.MatchString(msg) {
			continue
		}

		out = re.ReplaceAllString(msg, v)
	}

	if out != "" {
		return out
	}

	return msg
}

// ReqLocale tries to return the locale from the request.
// It tries to retrieve it first using the "lang" cookie and otherwise
// using the "Accept-Language" request header. If the locale is not recognized
// or not supported, it will return the default locale (en_US).
func (tr *Translator) ReqLocale(r *http.Request) string {
	lang, err := r.Cookie("lang")
	if err == nil {
		if _, ok := tr.locales[lang.Value]; ok {
			return lang.Value
		} else if locale, err := tr.localeFromLang(lang.Value); err == nil {
			return locale
		}
	}

	langs, _, err := parseAcceptLanguage(r.Header.Get("Accept-Language"))
	if err == nil {
		for _, lang := range langs {
			if _, ok := tr.locales[lang]; ok {
				return lang
			} else if locale, err := tr.localeFromLang(lang); err == nil {
				return locale
			}
		}
	}

	return defaultLocale
}

// parseAcceptLanguage parses a Accept-Language http header.
func parseAcceptLanguage(s string) (langs []string, q []float32, err error) {
	var entry string
	for s != "" {
		if entry, s = split(s, ','); entry == "" {
			continue
		}

		lang, weight := split(entry, ';')
		langs = append(langs, lang)

		// Scan the optional weight.
		w := 1.0
		if weight != "" {
			weight = consume(weight, 'q')
			weight = consume(weight, '=')
			// consume returns the empty string when a token could not be
			// consumed, resulting in an error for ParseFloat.
			if w, err = strconv.ParseFloat(weight, 32); err != nil {
				return nil, nil, err
			}
			// Drop tags with a quality weight of 0.
			if w <= 0 {
				continue
			}
		}

		q = append(q, float32(w))
	}

	sort.Stable(&qSort{langs, q})
	return langs, q, nil
}

// consume removes a leading token c from s and returns the result or the empty
// string if there is no such token.
func consume(s string, c byte) string {
	if s == "" || s[0] != c {
		return ""
	}
	return strings.TrimSpace(s[1:])
}

// split splits a string at character c into a head and tail and returns
// the full string as the head if not found.
func split(s string, c byte) (head, tail string) {
	if i := strings.IndexByte(s, c); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s), ""
}

// qSort implements sort.Interface to sort a list
// of languages according to q indexes.
type qSort struct {
	langs []string
	q     []float32
}

func (s *qSort) Len() int {
	return len(s.q)
}

func (s *qSort) Less(i, j int) bool {
	return s.q[i] > s.q[j]
}

func (s *qSort) Swap(i, j int) {
	s.langs[i], s.langs[j] = s.langs[j], s.langs[i]
	s.q[i], s.q[j] = s.q[j], s.q[i]
}

// langFromLocale returns the language part of a locale.
func (tr *Translator) langFromLocale(locale string) string {
	return strings.Split(locale, "_")[0]
}

// localeFromLang returns the first locale matching the given lang.
func (tr *Translator) localeFromLang(lang string) (string, error) {
	for locale := range tr.locales {
		if strings.HasPrefix(locale, lang+"_") {
			return locale, nil
		}
	}

	return "", fmt.Errorf("no locale found for language %s", lang)
}

// Format uses the package monday to format a time.Time according to a locale
// with month and days translated.
func Format(dt time.Time, layout string, locale string) string {
	return monday.Format(dt, layout, monday.Locale(locale))
}
