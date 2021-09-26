def basic_fetch_url(url):
    return _derivation(
        name=url.split("/")[-1], builder="basic_fetch_url", env={"url": url}
    )


def busybox():
    b = basic_fetch_url("https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz")
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


def new_fetch_url(name, env):
    fetched = basic_fetch_url("https://brmbl.s3.amazonaws.com/url_fetcher.tar.gz")

    b = busybox()
    url_fetcher = _derivation(
        name="url_fetcher",
        builder=b.out + "/bin/sh",
        args=[
            "-c",
            """
		set -ex
		mkdir $out/bin
		cp $fetched/url_fetcher $out/bin/url_fetcher
		chmod +x $out/bin/url_fetcher
		""",
        ],
        env={"PATH": b.out + "/bin", "fetched": fetched},
    )
    return _derivation(
        name,
        url_fetcher.out + "/bin/url_fetcher",
        env=env,
        network=True,
        _internal_key=internal_key,
    )


def cacerts():
    """cacerts provides known certificate authority certificates to verify TLS connections"""
    return _derivation(
        name="ca-certificates",
        builder=busybox().out + "/bin/sh",
        env=dict(
            PATH=busybox().out + "/bin",
            src=new_fetch_url(
                "ca-certificates.crt",
                {"url": "https://brmbl.s3.amazonaws.com/ca-certificates.crt"},
            ),
        ),
        args=[
            "-c",
            """
            set -ex
            cp -r $src/ca-certificates.crt $out
            cp $out/ca-certificates.crt $out/ca-bundle.crt
            """,
        ],
    )


def git(name, env):
    src = new_fetch_url(
        "git-2.10.2-1.tar.gz",
        dict(url="http://s.minos.io/archive/bifrost/x86_64/git-2.10.2-1.tar.gz"),
    )
    git = _derivation(
        name="git",
        builder=busybox().out + "/bin/sh",
        args=[
            "-c",
            """
            set -ex
            cp -r $src/usr/* $out
            mkdir test
            cd test
            $out/bin/git --version
            """,
        ],
        env=dict(src=src, PATH=busybox().out + "/bin"),
    )
    return _derivation(
        name="git_fetcher",
        builder=busybox().out + "/bin/sh",
        args=[
            "-c",
            """
        git clone $url $out
        rm -rf $out/.git
        """,
        ],
        network=True,
        _internal_key=internal_key,
        env=dict(
            git=git,
            PATH=git.out + "/bin:" + busybox().out + "/bin",
            GIT_EXEC_PATH=git.out + "/libexec/git-core",
            GIT_SSL_CAINFO=cacerts().out + "/ca-certificates.crt",
            url=env.get("url"),
        ),
    )


def derivation(name, builder, env={}, **kwargs):
    if builder == "fetch_url":
        return new_fetch_url(name, env)
    if builder == "fetch_git":
        return git(name, env)
    return _derivation(name, builder, env=env, **kwargs)
