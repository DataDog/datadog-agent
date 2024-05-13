package language

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"go.uber.org/zap"
)

type Language string

const (
	Unknown   Language = "UNKNOWN"
	Java               = "jvm"
	Node               = "nodejs"
	Python             = "python"
	Ruby               = "ruby"
	DotNet             = "dotnet"
	Go                 = "go"
	CPlusPlus          = "cpp"
	PHP                = "php"
)

var (
	procToLanguage = map[string]Language{
		"java":    Java,
		"node":    Node,
		"nodemon": Node,
		"python":  Python,
		"python3": Python,
		"dotnet":  DotNet,
		"ruby":    Ruby,
		"bundle":  Ruby,
	}
)

func (lf Finder) Detect(args []string, envs []string) (Language, bool) {
	lang := lf.findLang(ProcessInfo{
		Args: args,
		Envs: envs,
	})
	if lang == "" {
		return Unknown, false
	}
	return lang, true
}

func findFile(fileName string) (io.ReadCloser, bool) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, false
	}
	return f, true
}

type ProcessInfo struct {
	Args []string
	Envs []string
}

func (pi ProcessInfo) FileReader() (io.ReadCloser, bool) {
	fileName := pi.Args[0]
	// if it's an absolute path, use it
	if strings.HasPrefix(fileName, "/") {
		return findFile(fileName)
	}
	for _, env := range pi.Envs {
		if key, val, _ := strings.Cut(env, "="); key == "PATH" {
			paths := strings.Split(val, ":")
			for _, path := range paths {
				if r, found := findFile(path + string(os.PathSeparator) + fileName); found {
					return r, true
				}
			}
		}
	}
	// well, just try it as a relative path, maybe it works
	return findFile(fileName)
}

type Matcher interface {
	Language() Language
	Match(pi ProcessInfo) bool
}

func New(l *zap.Logger) Finder {
	return Finder{
		Logger: l,
		Matchers: []Matcher{
			PythonScript{},
			RubyScript{},
			DotNetBinary{},
		},
	}
}

type Finder struct {
	Logger   *zap.Logger
	Matchers []Matcher
}

func (lf Finder) findLang(pi ProcessInfo) Language {
	lang := findInArgs(pi.Args)
	lf.Logger.Debug("language found", zap.Any("lang", lang))

	// if we can't figure out a language from the command line, try alternate methods
	if lang == "" {
		lf.Logger.Debug("trying alternate methods")
		for _, matcher := range lf.Matchers {
			if matcher.Match(pi) {
				lf.Logger.Debug(fmt.Sprintf("%T matched", matcher))
				lang = matcher.Language()
				break
			}
		}
	}
	return lang
}

func findInArgs(args []string) Language {
	// empty slice passed in
	if len(args) == 0 {
		return ""
	}
	for i := 0; i < len(args); i++ {
		procName := path.Base(args[i])
		// if procName is a known language, return the pos and the language
		if lang, ok := procToLanguage[procName]; ok {
			return lang
		}
	}
	return ""
}
