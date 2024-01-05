# ISSUES

No issues allowed on [GitLab repo](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/geopki).

## `./cmd/db-address-exporter` Runs Out Of Memory

`./cmd/db-address-exporter` runs out of memory. It slowely but surely consumes whatever memory (and swap) the system offers.
Then, it operates mostly at max mem capacity. Sometimes it frees a few GBs or so, only to reclaim them.
Eventually, the program crashes. Most probably the OS needed some memory somewhere, meaning it shut down the most memory intensive process.

Reproduction is sketchy, it fails after a varying amount of percentage of completed work.

It is not clear for what this amount of memory is used. The `.parquet` file is roughly 1GB, uncompressed at a guess 8GB.

I tried with 24 GB of memory (some was swap).

## Queries to Empty Tree Fail

failure message: `integrity check failed, root node was not returned by the queryintegrity check failed, root node was not returned by the query`

somehow the edge case of empty tree is not handled

## Web Demo Non-Empty Query Shows Error

Pop up with `X is null`. Formerly `t is null`, after some of my commits it is now `str is null`.

The only null thing is the second entry in `GeoCertArea.coordinates`, which should be of type `List<LinearRing>` (LinearRing is `List<List<Float>>`).

Console does not log anything.

Empty queries don't have this.

## (fixed) CORS not Working

Noticable only in Web Demo client.

Bug in code.

Fixed in f30c80e.

## (fixed) requirements.txt Missing PyArrow

fixed in 1385c00

## (fixed) README Instructions Incomplete

addressed in 01849cc & continuously
