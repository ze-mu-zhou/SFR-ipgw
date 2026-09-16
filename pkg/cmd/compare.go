package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"github.com/urfave/cli/v2"
)

var CompareCommand = &cli.Command{
	Name: "compare", Usage: "compare two saved traffic snapshots without accessing the network", ArgsUsage: "before.json after.json",
	Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "write JSON result"}},
	Action: func(ctx *cli.Context) error {
		if ctx.NArg() != 2 {
			return errors.New("用法：ipgw compare [--json] before.json after.json")
		}
		before, err := readSnapshot(ctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("读取前快照失败：%w", err)
		}
		after, err := readSnapshot(ctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("读取后快照失败：%w", err)
		}
		result, err := model.CompareSnapshots(before, after)
		if err != nil {
			return err
		}
		if ctx.Bool("json") {
			enc := json.NewEncoder(ctx.App.Writer)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Fprintf(ctx.App.Writer, "账号：%s\n计费周期：%s\n显示用量差值：约 %d 字节\n显示精度保守误差：±%d 字节\n%s\n", result.AccountID, result.BillingPeriod, result.DeltaBytes, result.UncertaintyBytes, result.Note)
		return nil
	},
	OnUsageError: onUsageError,
}

func readSnapshot(path string) (model.Snapshot, error) {
	var s model.Snapshot
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	const limit = 1024 * 1024
	stat, err := f.Stat()
	if err != nil {
		return s, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > limit {
		return s, errors.New("快照必须是小于 1 MiB 的普通 JSON 文件")
	}
	dec := json.NewDecoder(io.LimitReader(f, limit+1))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&s); err != nil {
		return s, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return s, errors.New("快照包含额外内容")
	}
	return s, nil
}
