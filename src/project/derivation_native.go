package project

import (
	"go.starlark.net/starlark"
)

var derivationModule = `

def fetch_url(url):
	return _derivation(name=url.split("/")[-1], builder="basic_fetch_url", env={"url": url})


def busybox():
    b = fetch_url("https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz")
    script = """
    set -e
    $busybox_download/busybox-x86_64 mkdir $out/bin
    $busybox_download/busybox-x86_64 cp $busybox_download/busybox-x86_64 $out/bin/busybox
    cd $out/bin
    for command in $(./busybox --list); do
        ./busybox ln -s busybox $command
    done
    """
    return _derivation(
        name="busybox",
        builder=b.out + "/busybox-x86_64",
        args=["sh", "-c", script],
        env={"busybox_download": b, "PATH": b.out},
    )


def derivation(name, builder, env={}, **kwargs):
	if builder == "fetch_url":
		fetched = fetch_url("https://brmbl.s3.amazonaws.com/url_fetcher.tar.gz")
		b = busybox()
		url_fetcher = _derivation(
			name="url_fetcher",
			builder=b.out + "/bin/sh",
			args=["-c", """
			set -ex
			mkdir $out/bin
			cp $fetched/url_fetcher $out/bin/url_fetcher
			chmod +x $out/bin/url_fetcher
			"""],
			env={"PATH": b.out+"/bin", "fetched": fetched},
		)
		return _derivation(name, url_fetcher.out + "/bin/url_fetcher", env=env, network=True, _internal_key=internal_key)

	return _derivation(name, builder, env=env, **kwargs)
`

// LoadAssertModule loads the assert module. It is concurrency-safe and
// idempotent.
func (rt *runtime) loadNativeDerivation(derivation starlark.Value) (starlark.Value, error) {
	predeclared := starlark.StringDict{
		"_derivation":  derivation,
		"internal_key": starlark.MakeInt64(rt.internalKey),
	}

	thread := new(starlark.Thread)
	globals, err := starlark.ExecFile(thread, "derivation.bramble", derivationModule, predeclared)
	if err != nil {
		return nil, err
	}
	return globals["derivation"], nil
}
