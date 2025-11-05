package util

import (
    "bufio"
    "fmt"
    "os"
    "strings"
)

func Prompt(label string) (string, error) {
    fmt.Print(label)
    reader := bufio.NewReader(os.Stdin)
    s, err := reader.ReadString('\n')
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(s), nil
}

func PromptOptional(label string) (string, error) {
    return Prompt(label)
}

func PromptWithDefault(label string, def string) (string, error) {
    if def != "" {
        label = fmt.Sprintf("%s [%s]: ", strings.TrimRight(label, ": "), def)
    }
    v, err := Prompt(label)
    if err != nil {
        return "", err
    }
    if strings.TrimSpace(v) == "" {
        return def, nil
    }
    return v, nil
}


