package model

import (
	"fmt"
)

type Info struct {
	Username          string
	IP                string
	Traffic, UsedTime int64
	Balance           float64
}

// FormattedTraffic 以十进制单位（1 KB = 1000 B）显示网关返回的字节数。
func (i *Info) FormattedTraffic() string {
	units := []struct {
		size int64
		name string
	}{{1000 * 1000 * 1000 * 1000, "TB"}, {1000 * 1000 * 1000, "GB"}, {1000 * 1000, "MB"}, {1000, "KB"}}
	for _, unit := range units {
		if i.Traffic >= unit.size {
			return fmt.Sprintf("%.2f %s", float64(i.Traffic)/float64(unit.size), unit.name)
		}
	}
	return fmt.Sprintf("%d B", i.Traffic)
}

func (i *Info) FormattedUsedTime() string {
	hours := i.UsedTime / 3600
	minutes := i.UsedTime % 3600 / 60
	seconds := i.UsedTime % 60
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
}

func (i *Info) FormattedBalance() string {
	return fmt.Sprintf("%.2f 元", i.Balance)
}
