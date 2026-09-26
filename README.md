# task26092711

会展展位分配库（Go + SQLite），包路径 `expobooth/booth`。

## 用法

```go
hall := booth.Hall{Rows: 10, Cols: 40, Blocked: []booth.Cell{{Row: 0, Col: 5}}}
db, _ := booth.Open("hall.db", hall)
defer db.Close()

res, _ := db.Request(t, "acme", booth.Island) // 立即发 offer 或排队
acc, _ := db.Accept(t, "acme")                // offer 转已售
kind, _ := db.Release(t, "acme")              // 撤队 / 拒 offer / 退已售
snap, _ := db.Snapshot(t)                     // 已售 + 有效 offer(含到期时刻) + 队列
```

- 网格行 A–Z、列 1 起，每格 10×10 英尺；Standard 1 格、Premium 同行相邻 2 格、Island 2×2；展位编号取左上角格（如 `C-14`）。
- 时间只来自参数 `t`（UTC），全局单调不减，回退报 `ErrTimeRegression`；不读系统时钟，无后台 goroutine。
- offer 自创建时刻起 48h 有效；到期/被拒即出局。有格子释放时按到达顺序扫一遍等候队列，放不下的不挡后面的；同一时刻到期的 offer 先合并释放再统一扫一次；`t` 恰等于到期时刻视为已过期。

## 设计

- **惰性结算**：状态只由命令序列决定。每次调用先在事务内把 `t` 之前所有到期批按真实时刻依次结算（释放 → 递补 → 新 offer 挂 48h 后再到期，链式推进），结算结果落库，因此任意插入 `Snapshot` 或关闭重开都不改变任何返回值。
- **增量维护**：结算循环在内存中维护占用位图（26×80 位掩码，首适应放置）与等候队列，每批只做定常次 SQL 写；单次调用可结算数千批积压（实测 2001 批约 75ms），开销不随历史命令总数增长。
- **持久化**：单文件 SQLite（WAL），表 `meta`（hall 配置、最大已见时间、序号）、`blocked`、`requests`（每参展商至多一条在途请求）。重开时 hall 必须一致，否则 `ErrHallMismatch`。

## 测试

```sh
go test ./booth/
```

含确定性对照测试（同一脚本跑两遍：纯命令 vs 穿插 `Snapshot` + 中途关闭重开，逐返回值比对）、链式过期、同时刻合并释放、位图边界与 2 秒积压结算性能测试。
