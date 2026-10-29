# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 硬性规定

1. 非必要情况下，用精炼的简体中文大白话。
2. 没有完整的测试验收，不许提交，不许推送。
3. 凡是涉及代码的改动，必须在本地新开 git branch + worktree 里做；得到用户确认后，才能在本地合并回 `dev`。

## 项目是什么

symphony：用 Claude Code 对标 grok bot 的试验仓库。独立开发，不参考其它仓库。

## 代码现状

还没有代码。目前只有 `README.md` 和一份 GitHub 的 Go 模板 `.gitignore`；技术栈还没定，不要因为模板就当成 Go 项目，先问用户。

尚无构建、测试命令，加代码后在这里补充。

## 分支

`dev` 是开发主线，代码改动按硬性规定第 3 条走。
