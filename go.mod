module github.com/tejaskumark/tftp/v2

go 1.23.0

retract (
	v1.0.1 // Contains retractions only.
	v1.0.0 // Published accidentally.
)

require golang.org/x/net v0.43.0

require golang.org/x/sys v0.35.0 // indirect
