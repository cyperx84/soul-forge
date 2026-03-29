package animation

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

var forgeFrames = []string{
	`
        *
       /|\
      / | \
     /  |  \
    /   |   \
  [=====*=====]
  |  SOUL-FORGE |
  [=============]
       |||
      _|||_
     |     |
     |_____|
`,
	`
     *   *
    / \ / \
   /   X   \
  /  * | *  \
 /    \|/    \
[======*======]
|  SOUL-FORGE |
[=============]
      |||
     _|||_
    |     |
    |_____|
`,
	`
    * * * * *
   *  \|/|/  *
  *  --*--   *
   * /|\ \  *
    *   *   *
[=============]
|  SOUL-FORGE |
[=============]
      |||
     _|||_
    |     |
    |_____|
`,
	`
       *
      *|*
     **|**
    * \|/ *
   *--[*]--*
[=============]
|  SOUL-FORGE |
[=============]
      |||
     _|||_
    |     |
    |_____|
`,
	`
  *         *
   *       *
    *     *
     * * *
      ***
[=============]
|  SOUL-FORGE |
[=============]
      |||
     _|||_
    |     |
    |_____|
`,
	`
[=============]
|  SOUL-FORGE |
[=============]
      |||
     _|||_
    |     |
    |_____|

  Forge ready.
`,
}

func PlayForge() {
	// Hide cursor
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	lineCount := 0
	for i, frame := range forgeFrames {
		if i > 0 {
			// Move cursor up by lineCount
			fmt.Printf("\033[%dA", lineCount)
		}
		fmt.Print(frame)
		lineCount = countLines(frame)
		if i < len(forgeFrames)-1 {
			time.Sleep(280 * time.Millisecond)
		}
	}
	fmt.Println()
}

func countLines(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}
