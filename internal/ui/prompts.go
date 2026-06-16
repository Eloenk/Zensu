package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var reader = bufio.NewReader(os.Stdin)

func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(strings.TrimRight(line, "\r\n")), err
}

func PromptAnimeName() (string, error) {
	for {
		name, err := readLine("  Search anime: ")
		if err != nil {
			return "", err
		}
		if name != "" {
			return name, nil
		}
		fmt.Println("  Please enter a name.")
	}
}

func PromptSelectAnime(titles []string) (int, error) {
	if len(titles) == 1 {
		return 0, nil
	}

	fmt.Println()
	for i, t := range titles {
		fmt.Printf("  [%d] %s\n", i+1, t)
	}
	fmt.Println()

	for {
		s, err := readLine(fmt.Sprintf("  Select [1-%d]: ", len(titles)))
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(s)
		if err == nil && n >= 1 && n <= len(titles) {
			return n - 1, nil
		}
		fmt.Printf("  Enter a number between 1 and %d.\n", len(titles))
	}
}

func PromptEpisodes(allEps []float64, existingMap map[float64]bool) ([]float64, error) {
	first := allEps[0]
	last := allEps[len(allEps)-1]

	fmt.Printf("\n  Available: E%.0f – E%.0f (%d episodes)\n", first, last, len(allEps))

	var existingList []float64
	for _, ep := range allEps {
		if existingMap[ep] {
			existingList = append(existingList, ep)
		}
	}

	if len(existingList) > 0 {
		var epStrs []string
		limit := len(existingList)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			epStrs = append(epStrs, fmt.Sprintf("E%.0f", existingList[i]))
		}
		if len(existingList) > 10 {
			epStrs = append(epStrs, "...")
		}
		fmt.Printf("  Found %d existing episodes in download folder: %s\n", len(existingList), strings.Join(epStrs, ", "))
	}
	fmt.Println()

	var promptStr string
	if len(existingList) > 0 {
		promptStr = "  Episodes (e.g. 5  or  1-12  or  0 to resume remaining): "
	} else {
		promptStr = "  Episodes (e.g. 5  or  1-12  or  * for all): "
	}

	for {
		s, err := readLine(promptStr)
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if s == "*" && len(existingList) > 0 {
			fmt.Println("  Option '*' is disabled since existing files were found. Use '0' to resume remaining.")
			continue
		}
		if s == "0" {
			if len(existingList) == 0 {
				fmt.Println("  No existing episodes found to resume. Use '*' to download all episodes.")
				continue
			}
			maxExisting := 0.0
			hasExisting := false
			for ep := range existingMap {
				if !hasExisting || ep > maxExisting {
					maxExisting = ep
					hasExisting = true
				}
			}

			var eps []float64
			for _, ep := range allEps {
				if !hasExisting || ep > maxExisting {
					eps = append(eps, ep)
				}
			}
			if len(eps) == 0 {
				fmt.Println("  All episodes are already downloaded!")
				continue
			}
			return eps, nil
		}

		eps, err := ParseEpisodeInput(s, allEps)
		if err != nil {
			fmt.Printf("  Invalid input: %v\n", err)
			continue
		}
		if len(eps) == 0 {
			fmt.Println("  No matching episodes found.")
			continue
		}
		return eps, nil
	}
}

func PromptResolution() (string, error) {
	fmt.Println("\n  Resolution:")
	opts := []string{"1080", "720", "480", "360"}
	for i, o := range opts {
		fmt.Printf("  [%d] %sp\n", i+1, o)
	}
	fmt.Println()

	for {
		s, err := readLine("  Choice [1-4] (default 1): ")
		if err != nil {
			return "", err
		}
		if s == "" {
			return "1080", nil
		}
		n, err := strconv.Atoi(s)
		if err == nil && n >= 1 && n <= 4 {
			return opts[n-1], nil
		}
		fmt.Println("  Enter 1, 2, 3, or 4.")
	}
}

func PromptAudio() (string, error) {
	fmt.Println("\n  Audio:")
	fmt.Println("  [1] Japanese (jpn)")
	fmt.Println("  [2] English dub (eng)")
	fmt.Println()

	for {
		s, err := readLine("  Choice [1/2] (default 1): ")
		if err != nil {
			return "", err
		}
		switch s {
		case "", "1":
			return "jpn", nil
		case "2":
			return "eng", nil
		}
		fmt.Println("  Enter 1 or 2.")
	}
}

func PromptDownloadDir(defaultDir string) (string, error) {
	s, err := readLine(fmt.Sprintf("\n  Download folder [%s]: ", defaultDir))
	if err != nil {
		return "", err
	}
	if s == "" {
		return defaultDir, nil
	}

	if strings.HasPrefix(s, "~/") {
		home, _ := os.UserHomeDir()
		s = home + s[1:]
	}
	return s, nil
}

func PromptConfirm(msg string) (bool, error) {
	s, err := readLine(fmt.Sprintf("\n  %s [Y/n]: ", msg))
	if err != nil {
		return false, err
	}
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "" || s == "y" || s == "yes", nil
}
