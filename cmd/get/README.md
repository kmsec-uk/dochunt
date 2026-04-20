# GET commoncrawl gdocs

GET commoncrawl Google Docs and insert them into the database.

This is the original CLI used to populate the `dochunt.kmsec.uk` dataset,
and should therefore be treated as the canonical reference implementation for
interacting with the database schema.

This utility leverages the commoncrawl index to first search a commoncrawl
dataset for archived webpages from Google Docs before downloading the WARC
items individually and inserting them into a sqlite database.

It is a single executable split into two files. It is recommended to run
this with the go experimental flag for the `encoding/json/v2` experiment,
as that is what I've used for my Google Docs parsing library, for example:

```bash
kmsec@penguin:~/cmd/get$ GOEXPERIMENT=jsonv2 go run *.go -h
Google Docs CommonCrawl scraper!
  -c string
        commoncrawl collection to scrape (default "CC-MAIN-2026-12")
  -db string
        path to database (default "../data/store.db")
```
However, there's a remote chance modifying source files to use the regular JSON
library would change behaviour.

