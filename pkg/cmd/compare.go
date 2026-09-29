package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var CompareCommand = &cli.Command{
	Name:      "compare",
	Usage:     "离线比较两份已保存的流量快照",
	ArgsUsage: "前快照.json 后快照.json",
	Flags:     []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "以 JSON 格式输出结果"}},
	Action: func(ctx *cli.Context) error {
		if err := rejectMisplacedOptions(ctx); err != nil {
			return err
		}
		if ctx.NArg() != 2 {
			return errors.New("用法：ipgw compare [--json] 前快照.json 后快照.json")
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
			encoder := json.NewEncoder(ctx.App.Writer)
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		}
		fmt.Fprintf(ctx.App.Writer, "账号：%s\n计费周期：%s\n显示用量差值：约 %d 字节\n显示精度保守误差：±%d 字节\n%s\n", result.AccountID, result.BillingPeriod, result.DeltaBytes, result.UncertaintyBytes, result.Note)
		return nil
	},
	OnUsageError: onUsageError,
}

func readSnapshot(path string) (model.Snapshot, error) {
	var snapshot model.Snapshot
	file, err := os.Open(path)
	if err != nil {
		return snapshot, err
	}
	defer file.Close()
	const limit = 1024 * 1024
	stat, err := file.Stat()
	if err != nil {
		return snapshot, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > limit {
		return snapshot, errors.New("快照必须是小于 1 MiB 的普通 JSON 文件")
	}
	decoder := json.NewDecoder(io.LimitReader(file, limit+1))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&snapshot); err != nil {
		return snapshot, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return snapshot, errors.New("快照包含额外内容")
	}
	return snapshot, nil
}
