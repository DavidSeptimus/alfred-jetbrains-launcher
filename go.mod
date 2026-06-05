module github.com/davidseptimus/alfred-jetbrains-launcher

go 1.23

// The task runner core is an independent module developed in this repo (see
// go.work). The replace keeps release builds resolving it from ./taskrunner
// without publishing; on extraction, drop the replace and require a tag.
require github.com/davidseptimus/alfred-taskrunner v0.0.0

replace github.com/davidseptimus/alfred-taskrunner => ./taskrunner
