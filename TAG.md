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

## Phase 2: Pool quantile.Agent and inner maps

```
goos: linux
goarch: amd64
pkg: github.com/DataDog/datadog-agent/pkg/aggregator
cpu: 12th Gen Intel(R) Core(TM) i9-12900H
BenchmarkFlushSketches_NoFilter_100-20             15072     78634 ns/op    83679 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16525     70901 ns/op    83615 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16686     72279 ns/op    83590 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16639     73076 ns/op    83618 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16195     74620 ns/op    83514 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16492     74116 ns/op    83729 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             15283     75083 ns/op    83631 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16239     75095 ns/op    83667 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16272     72939 ns/op    83677 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             15919     75305 ns/op    83691 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1371    775826 ns/op   841642 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1419    776542 ns/op   841369 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1593    782054 ns/op   840604 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1555    823715 ns/op   842197 B/op  11918 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1600    772410 ns/op   841718 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1525    764502 ns/op   840451 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1556    778439 ns/op   841998 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1539    762268 ns/op   840300 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1485    774785 ns/op   840390 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1491    764116 ns/op   840083 B/op  11917 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13160     90183 ns/op    88548 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12745     93316 ns/op    88617 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   12858     94632 ns/op    88641 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13086     90316 ns/op    88613 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13183     89681 ns/op    88603 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13708     89311 ns/op    88517 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13260     88965 ns/op    88634 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13425     89897 ns/op    88472 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13041     90499 ns/op    88608 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   13251     88742 ns/op    88564 B/op   1309 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 15372     79046 ns/op    61373 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 15560     75575 ns/op    61394 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14698     91087 ns/op    61451 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14118     82348 ns/op    61353 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14232     82629 ns/op    61336 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14296     80909 ns/op    61376 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14919     81984 ns/op    61411 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14878     80056 ns/op    61335 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 15021     81285 ns/op    61404 B/op   1104 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 14829     82818 ns/op    61451 B/op   1104 allocs/op
```

**Phase 2 vs Phase 1:**
- NoFilter_100: 1320→1109 allocs/op (-16%), 101KB→83KB B/op (-18%)
- NoFilter_1000: 13933→11917 allocs/op (-14%), 1042KB→841KB B/op (-19%)
- Filter_NoCollision_100: 1520→1309 allocs/op (-14%), 106KB→88KB B/op (-17%)
- Filter_HighCollision_100: 1315→1104 allocs/op (-16%), 79KB→61KB B/op (-22%)

## Phase 3: Cache stripped key on Context

```
goos: linux
goarch: amd64
pkg: github.com/DataDog/datadog-agent/pkg/aggregator
cpu: 12th Gen Intel(R) Core(TM) i9-12900H
BenchmarkFlushSketches_NoFilter_100-20             16444     76854 ns/op    83687 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16286     71870 ns/op    83490 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             17140     71641 ns/op    83521 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16795     71282 ns/op    83573 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16628     72379 ns/op    83522 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16821     71945 ns/op    83497 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             15904     72879 ns/op    83562 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16804     72283 ns/op    83522 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             16495     74375 ns/op    83584 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_100-20             17023     75912 ns/op    83702 B/op   1109 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1582    753783 ns/op   839997 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1491    771828 ns/op   841385 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1570    766237 ns/op   842039 B/op  11918 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1635    761884 ns/op   841106 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1592    769254 ns/op   841207 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1622    774269 ns/op   840250 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1556    771830 ns/op   841983 B/op  11918 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1636    765430 ns/op   841276 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1549    775375 ns/op   840902 B/op  11917 allocs/op
BenchmarkFlushSketches_NoFilter_1000-20             1587    758426 ns/op   839947 B/op  11917 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15656     78132 ns/op    86810 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15674     77276 ns/op    86846 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15546     78424 ns/op    86893 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15139     77173 ns/op    86869 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15344     78671 ns/op    86859 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   14870     78205 ns/op    86989 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15272     78407 ns/op    86879 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   14944     80924 ns/op    86945 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15099     79675 ns/op    86872 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_NoCollision_100-20   15334     80137 ns/op    86841 B/op   1209 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 17923     64989 ns/op    59653 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 17804     65203 ns/op    59690 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18524     65438 ns/op    59661 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18092     66416 ns/op    59660 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18226     66147 ns/op    59704 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18224     65418 ns/op    59690 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18694     64849 ns/op    59713 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 17797     66695 ns/op    59660 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18544     65419 ns/op    59682 B/op   1004 allocs/op
BenchmarkFlushSketches_Filter_HighCollision_100-20 18882     65041 ns/op    59739 B/op   1004 allocs/op
```

**Phase 3 vs Phase 2:**
- NoFilter_100: 1109→1109 allocs/op (no change — tag filter not applied)
- NoFilter_1000: 11917→11917 allocs/op (no change — tag filter not applied)
- Filter_NoCollision_100: 1309→1209 allocs/op (-8%), 88KB→86KB B/op (-2%)
- Filter_HighCollision_100: 1104→1004 allocs/op (-9%), 61KB→59KB B/op (-3%)

## Summary: Baseline vs Final

| Benchmark | Baseline allocs | Final allocs | Reduction | Baseline B/op | Final B/op | Reduction |
|---|---|---|---|---|---|---|
| NoFilter_100 | 1438 | 1109 | -23% | 125KB | 83KB | -34% |
| NoFilter_1000 | 14968 | 11917 | -20% | 1426KB | 841KB | -41% |
| Filter_NoCollision_100 | 1638 | 1209 | -26% | 130KB | 86KB | -34% |
| Filter_HighCollision_100 | 1433 | 1004 | -30% | 103KB | 59KB | -43% |
