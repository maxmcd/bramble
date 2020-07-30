package bramblescript

import (
	"testing"
)

func TestStarlarkSesssion(t *testing.T) {
	tests := []scriptTest{
		{script: `b=session()`,
			returnValue: `<session ''>`},
		{script: `b=[getattr(session(), x) for x in dir(session())]`,
			returnValue: ``},
		{script: `b=session().env()`,
			returnValue: ``},
		{script: `b=session().env(1)`,
			returnValue: ``},
	}
	runTest(t, tests)
}
