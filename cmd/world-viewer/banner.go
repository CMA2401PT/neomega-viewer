package main

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// version 显示在终端开场和网页面板上，由发布标签在编译时注入：
//
//	go build -ldflags "-X main.version=v0.0.10"
//
// 直接 go build 时保持 dev，避免本地构建冒充某个已发布的版本。
var version = "dev"

// 开场画面与网页默认场景是同一幅画：海面升起的莫比乌斯环，扭转处收成一条细线。
const (
	bannerWidth  = 76
	bannerHeight = 21
	// 海平线所在行，以上是环，以下是倒影。
	bannerHorizon = 14
)

// bannerCell 是一格字符及其 256 色编号，0 表示不着色。
type bannerCell struct {
	char  rune
	color int
}

func renderBanner() [][]bannerCell {
	// 坐标以环半径为单位，终端字符高宽约为 2:1，所以 y 方向每行跨两倍距离。
	const (
		radius  = 1.0
		width   = 0.34
		centerY = 0.5
		twist   = math.Pi / 7
		unit    = 4.6 / bannerWidth
	)
	ramp := []rune(" .:-=+*#%@")
	grid := make([][]bannerCell, bannerHeight)
	for row := range grid {
		grid[row] = make([]bannerCell, bannerWidth)
		for col := range grid[row] {
			x := (float64(col) - bannerWidth/2 + 0.5) * unit
			y := float64(bannerHorizon-row) * unit * 2
			grid[row][col] = bannerCell{char: ' '}
			mirrored := y < 0
			if mirrored {
				// 倒影：镜像后随行号左右摆动，越远离海平线摆得越厉害，也越暗。
				depth := float64(row - bannerHorizon)
				x += 0.07 * depth * math.Sin(depth*1.7+x*2.4)
				y = -y
			}
			angle := math.Atan2(y-centerY, x)
			// 带子随圆周转半圈，正面看到的宽度按扭转相位变化，扭转点只剩一条线。
			band := width * math.Max(math.Sqrt(math.Abs(math.Sin((angle-twist)/2))), 0.2)
			distance := math.Abs(math.Hypot(x, y-centerY)-radius) / (band / 2)
			if distance < 1 {
				level := 1 - distance*distance
				// 环从水线的暖色升到顶端的冰白。
				rise := math.Min(math.Max((y)/(centerY+radius), 0), 1)
				palette := []int{209, 215, 223, 230, 195, 159}
				color := palette[int(rise*float64(len(palette)-1)+0.5)]
				if mirrored {
					depth := float64(row-bannerHorizon) / float64(bannerHeight-bannerHorizon)
					color = []int{153, 111, 74, 67, 60}[min(int(depth*5), 4)]
					// 倒影只用横向的笔画，像被波纹切成一段段的光。
					if math.Sin(float64(col)*0.9+float64(row)*2.3) > 0.55 {
						continue
					}
					grid[row][col] = bannerCell{char: []rune("-~=≈")[min(int(level*(1-0.5*depth)*4), 3)], color: color}
					continue
				}
				grid[row][col] = bannerCell{char: ramp[1+int(level*float64(len(ramp)-2)+0.5)], color: color}
				continue
			}
			switch {
			case row == bannerHorizon:
				// 海平线中间亮、两端熄灭。
				fade := math.Abs(x) / 2.2
				if fade < 1 {
					grid[row][col] = bannerCell{char: []rune("━─╌")[min(int(fade*3), 2)], color: []int{230, 152, 67}[min(int(fade*3), 2)]}
				}
			case mirrored:
				// 远离倒影的海面只留稀疏的涟漪。
				depth := float64(row - bannerHorizon)
				ripple := math.Sin(float64(col)*0.23+depth*2.1) + 0.8*math.Sin(float64(col)*0.07-depth*1.3)
				if ripple > 1.2-0.06*depth && math.Abs(x) < 2.25-0.08*depth {
					grid[row][col] = bannerCell{char: '-', color: []int{67, 60, 59, 238}[min(int(depth/2), 3)]}
				}
			}
		}
	}
	return grid
}

// printBanner 在终端画出开场，随后是致谢和网页地址。
// colored 为假时（输出被重定向）只写纯文本。
func printBanner(out io.Writer, address string, colored bool) {
	paint := func(code, text string) string {
		if !colored {
			return text
		}
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, row := range renderBanner() {
		// 只在颜色变化时写转义，行尾的空白不输出。
		end := len(row)
		for end > 0 && row[end-1].char == ' ' {
			end--
		}
		b.WriteString("  ")
		current := 0
		for _, cell := range row[:end] {
			if colored && cell.color != 0 && cell.color != current {
				fmt.Fprintf(&b, "\x1b[38;5;%dm", cell.color)
				current = cell.color
			}
			b.WriteRune(cell.char)
		}
		if current != 0 {
			b.WriteString("\x1b[0m")
		}
		b.WriteString("\n")
	}
	gray := func(text string) { b.WriteString("  " + paint("38;5;245", text) + "\n") }
	b.WriteString("\n")
	b.WriteString("  " + paint("1;38;5;230", "neOmega Viewer") + " " + paint("38;5;245", version) + "\n\n")
	gray("本项目未直接使用以下项目的代码，但许多关键数据来自它们：")
	gray("  Amulet      https://www.amuletmc.com/")
	gray("  GeyserMC    https://geysermc.org/")
	gray("  MC-Assets   https://github.com/zardoy/mc-assets")
	gray("为尊重 Amulet 的版权，版本支持停在 Minecraft 1.21。")
	gray("neOmega Viewer 是开源项目：https://github.com/CMA2401PT/neomega-viewer")
	b.WriteString("\n  " + paint("1", "打开 "+address+" 以使用 WebUI") + "\n\n")
	io.WriteString(out, b.String())
}

// browserAddress 把监听地址换成浏览器能直接打开的网址；监听所有地址时使用本机回环。
func browserAddress(listener net.Addr) string {
	host, port, err := net.SplitHostPort(listener.String())
	if err != nil {
		return "http://" + listener.String()
	}
	if ip := net.ParseIP(host); host == "" || ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// openBrowser 请求系统用默认浏览器打开网址。失败不影响服务，终端里已经印出了地址。
func openBrowser(address string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	if command.Start() == nil {
		go command.Wait()
	}
}

// terminalColors 判断标准输出是否为可以显示颜色的终端。
func terminalColors() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	stat, err := os.Stdout.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0 && enableTerminalColors()
}
