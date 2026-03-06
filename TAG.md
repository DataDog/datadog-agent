# TimeSampler Optimization Benchmarks

## Baseline (before any changes)

```
goos: linux
goarch: amd64
pkg: github.com/DataDog/datadog-agent/pkg/aggregator
cpu: 12th Gen Intel(R) Core(TM) i9-12900H
BenchmarkFlushSketches_NoFilter_100-20             12812     94623 ns/op   125671 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13454     89307 ns/op   125706 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13490     88790 ns/op   125612 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13652     86931 ns/op   125649 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13560     87833 ns/op   125582 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13656     87691 ns/op   125496 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13789     89956 ns/op   125619 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13566     87470 ns/op   125501 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13599     89928 ns/op   125559 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             13500     89349 ns/op   125611 B/op   1438 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1257    936282 ns/op  1425862 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1233    932692 ns/op  1425447 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1302    953604 ns/op  1426088 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1310   1012419 ns/op  1426534 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1300    971419 ns/op  1426555 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1309    967751 ns/op  1427382 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1222    943270 ns/op  1426030 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1257    993602 ns/op  1426516 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1046   1014074 ns/op  1426250 B/op  14968 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1220    982884 ns/op  1427258 B/op  14968 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    106236 ns/op   130532 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    102775 ns/op   130508 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    103675 ns/op   130485 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   11337    103268 ns/op   130385 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    104921 ns/op   130437 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    103446 ns/op   130354 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   11504    103865 ns/op   130432 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20    9499    105682 ns/op   130449 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    103891 ns/op   130558 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   10000    107202 ns/op   130584 B/op   1638 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12808     93465 ns/op   103683 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12823     93732 ns/op   103558 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12868     90774 ns/op   103645 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 13263     92976 ns/op   103716 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12969     92676 ns/op   103546 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12932     92527 ns/op   103592 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12853     92294 ns/op   103641 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12907     95571 ns/op   103679 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12853     95148 ns/op   103771 B/op   1433 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 12908     94824 ns/op   103722 B/op   1433 allocs/op
```

## Phase 1: Eliminate pointsByCtx + reuse maps

```
goos: linux
goarch: amd64
pkg: github.com/DataDog/datadog-agent/pkg/aggregator
cpu: 12th Gen Intel(R) Core(TM) i9-12900H
BenchmarkFlushSketches_NoFilter_100-20             12862     92349 ns/op   101806 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             12651     80871 ns/op   101803 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14250     84433 ns/op   101759 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14077     82662 ns/op   101695 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14701     83910 ns/op   101874 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14371     82875 ns/op   101778 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14671     81764 ns/op   101700 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14516     83060 ns/op   101765 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14362     84602 ns/op   101852 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             14350     84014 ns/op   101784 B/op   1320 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1418    859449 ns/op  1041884 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1383    868647 ns/op  1041755 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1347    880113 ns/op  1041990 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1381    839301 ns/op  1042092 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1368    872736 ns/op  1042088 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1431    887574 ns/op  1043047 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1419    857633 ns/op  1042088 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1404    848542 ns/op  1041781 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1464    853620 ns/op  1041269 B/op  13933 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1329    858002 ns/op  1041403 B/op  13933 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12448     95828 ns/op   106648 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12360     97347 ns/op   106668 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12154     95998 ns/op   106654 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12583     97322 ns/op   106670 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12321     97026 ns/op   106689 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12375     97620 ns/op   106731 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12200     96383 ns/op   106659 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12489    100648 ns/op   106819 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12238     98127 ns/op   106674 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12014     97901 ns/op   106740 B/op   1520 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14498     83016 ns/op    79626 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14166     84921 ns/op    79577 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 13740     86128 ns/op    79660 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14379     84014 ns/op    79614 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14538     85196 ns/op    79581 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14108     81699 ns/op    79567 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14218     83894 ns/op    79619 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14242     84911 ns/op    79582 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14362     82717 ns/op    79583 B/op   1315 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 13525     84470 ns/op    79614 B/op   1315 allocs/op
```

**Phase 1 vs Baseline:**
- NoFilter_100: 1438→1320 allocs/op (-8%), 125KB→101KB B/op (-19%)
- NoFilter_1000: 14968→13933 allocs/op (-7%), 1426KB→1042KB B/op (-27%)
- Filter_NoCollision_100: 1638→1520 allocs/op (-7%), 130KB→106KB B/op (-18%)
- Filter_HighCollision_100: 1433→1315 allocs/op (-8%), 103KB→79KB B/op (-23%)
