
# ![image](https://raw.githubusercontent.com/jdrews/logstation/master/web/public/favicon-32x32.png)  go-tailer #

go-tailer is a Go library designed to help you tail files in a similar fashion to `tail -f`   
  
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/jdrews/go-tailer)](https://pkg.go.dev/github.com/jdrews/go-tailer)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=jdrews_go-tailer&metric=security_rating)](https://sonarcloud.io/project/overview?id=jdrews_go-tailer)
[![Go Report Card](https://goreportcard.com/badge/github.com/jdrews/go-tailer)](https://goreportcard.com/report/github.com/jdrews/go-tailer)
![Build/Test](https://github.com/jdrews/go-tailer/actions/workflows/build-test.yml/badge.svg)


Built off the wonderful work by [@fstab](https://github.com/fstab) in the [grok_exporter/tailer module](https://github.com/fstab/grok_exporter/tree/master/tailer).

See the [FOSDEM17: Implementing 'tail -f'](https://www.youtube.com/watch?v=oc_iJXmUmrA) presentation for how go-tailer works.

# Usage
## Basic Usage: File Tailer
Here's a basic example of how to use this library to tail log files. 
```go
path := "/var/log/messages" // a single file
path := "/usr/local/myapp/logs/" // a directory
path = "C:\\Program Files\\MyApp\\logs\\*.log" // or a file wildcard

// parse the path glob
parsedGlob, err := glob.Parse(path)
if err != nil {
    panic(fmt.Sprintf("%q: failed to parse glob: %q", parsedGlob, err))
}

// startup a logrus logger
logger := logrus.New()

// startup the file tailer. RunFileTailer can take many path globs
tailer, err := fswatcher.RunFileTailer([]glob.Glob{parsedGlob}, false, true, logger)

// listen to the go channel for captured lines and do something with them
for line := range tailer.Lines() {
    // line.Line contains the line contents
    // line.File contains the name of the file that the line was grabbed from
    DoSomethingWithLine(line.File, line.Line)
}
```

## Polling Tailer
We recommend using the [RunFileTailer](https://github.com/jdrews/go-tailer/blob/6f5ab8f01f5db115fcb1bd72fdea19205a364910/fswatcher/fswatcher.go#L98), which listens for file system events to trigger tailing actions. But if that isn't working for you, you can fall back to a polling listener to periodically read the file for any new log lines. 
```go
// specify how often you want the tailer to check for updates
pollInterval := time.Duration(500 * time.Millisecond)

// startup the polling file tailer
tailer, err := fswatcher.RunPollingFileTailer([]glob.Glob{parsedGlob}, false, true, pollInterval, logger)

// listen to the go channel for captured lines and do something with them
for line := range tailer.Lines() {
    DoSomethingWithLine(line.File, line.Line)
}
```

## Other Tailers
Along with reading from files, go-tailer can read from other sources as well.
* Tail stdin (console/shell/standard input): [RunStdinTailer](https://github.com/jdrews/go-tailer/blob/main/stdinTailer.go)
* Tail a Kafka stream: [RunKafkaTailer](https://github.com/jdrews/go-tailer/blob/main/kafkaTailer.go)
* Tail a webhook: [WebhookTailer](https://github.com/jdrews/go-tailer/blob/main/webhookTailer.go)

# Frequently Asked Questions (FAQ)
### Why was the tailer module forked from [fstab/grok_exporter](https://github.com/fstab/grok_exporter) and moved here? 
grok_exporter had not been updated for 3-4 years and seems to be abandoned by [@fstab](https://github.com/fstab). It also was suffering [from quite a few security issues (CVEs) ](https://deps.dev/go/github.com%2Ffstab%2Fgrok_exporter/v0.2.8), which I've fixed. Additionally, the tailer module was not well known, had poor documentation, and I thought a separate repo would shed some well deserved light on it.   
   
I'd welcome making upstream patches to [fstab/grok_exporter](https://github.com/fstab/grok_exporter) if/when it becomes active again! 
