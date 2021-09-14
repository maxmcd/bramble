package bramble

import (
	"context"
	"encoding/json"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"
)

func (b bramble) runGit(ctx context.Context, opts build.RunDerivationOptions) (err error) {
	execOutputJSON := `{"Output":{"mdiesub7huad35wviydncxlfhffgsvai":{"Args":["-c",""],"Builder":"{{ qqfse23hmwjw4v3rw2yoa6l7cdgcobda:out }}/bin/sh","Dependencies":[{"Hash":"2y4sfa6sdh5a4wcqoyikt4m3i2zzukgs","Output":"out"},{"Hash":"qqfse23hmwjw4v3rw2yoa6l7cdgcobda","Output":"out"},{"Hash":"ty3hywqnh56vbn26543ca2mo7ucthqug","Output":"out"}],"Env":{"GIT_EXEC_PATH":"{{ ty3hywqnh56vbn26543ca2mo7ucthqug:out }}/libexec/git-core","GIT_SSL_CAINFO":"{{ 2y4sfa6sdh5a4wcqoyikt4m3i2zzukgs:out }}/ca-certificates.crt","PATH":"{{ ty3hywqnh56vbn26543ca2mo7ucthqug:out }}/bin","git":"{{ ty3hywqnh56vbn26543ca2mo7ucthqug:out }}"},"Name":"git_fetcher","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}}},"AllDerivations":{"23kutq6mumi6pnb4dbkt44vkgca73yiv":{"Args":null,"Builder":"fetch_url","Dependencies":null,"Env":{"url":"https://brmbl.s3.amazonaws.com/ca-certificates.crt"},"Name":"ca-certificates.crt","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"2oscpeuv4wsscndhcbxero2sabmdbzn4":{"Args":null,"Builder":"fetch_url","Dependencies":null,"Env":{"url":"https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz"},"Name":"busybox-x86_64.tar.gz","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"2y4sfa6sdh5a4wcqoyikt4m3i2zzukgs":{"Args":["-c","\n            set -ex\n            cp -r $src/ca-certificates.crt $out\n            cp $out/ca-certificates.crt $out/ca-bundle.crt\n            "],"Builder":"{{ qqfse23hmwjw4v3rw2yoa6l7cdgcobda:out }}/bin/sh","Dependencies":[{"Hash":"23kutq6mumi6pnb4dbkt44vkgca73yiv","Output":"out"},{"Hash":"qqfse23hmwjw4v3rw2yoa6l7cdgcobda","Output":"out"}],"Env":{"PATH":"{{ qqfse23hmwjw4v3rw2yoa6l7cdgcobda:out }}/bin","src":"{{ 23kutq6mumi6pnb4dbkt44vkgca73yiv:out }}"},"Name":"ca-certificates","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"mdiesub7huad35wviydncxlfhffgsvai":{"Args":["-c",""],"Builder":"{{ qqfse23hmwjw4v3rw2yoa6l7cdgcobda:out }}/bin/sh","Dependencies":[{"Hash":"2y4sfa6sdh5a4wcqoyikt4m3i2zzukgs","Output":"out"},{"Hash":"qqfse23hmwjw4v3rw2yoa6l7cdgcobda","Output":"out"},{"Hash":"ty3hywqnh56vbn26543ca2mo7ucthqug","Output":"out"}],"Env":{"GIT_EXEC_PATH":"{{ ty3hywqnh56vbn26543ca2mo7ucthqug:out }}/libexec/git-core","GIT_SSL_CAINFO":"{{ 2y4sfa6sdh5a4wcqoyikt4m3i2zzukgs:out }}/ca-certificates.crt","PATH":"{{ ty3hywqnh56vbn26543ca2mo7ucthqug:out }}/bin","git":"{{ ty3hywqnh56vbn26543ca2mo7ucthqug:out }}"},"Name":"git_fetcher","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"qqfse23hmwjw4v3rw2yoa6l7cdgcobda":{"Args":["sh","-c","\n    set -e\n    $busybox_download/busybox-x86_64 mkdir $out/bin\n    $busybox_download/busybox-x86_64 cp $busybox_download/busybox-x86_64 $out/bin/busybox\n    cd $out/bin\n    for command in $(./busybox --list); do\n        ./busybox ln -s busybox $command\n    done\n    "],"Builder":"{{ 2oscpeuv4wsscndhcbxero2sabmdbzn4:out }}/busybox-x86_64","Dependencies":[{"Hash":"2oscpeuv4wsscndhcbxero2sabmdbzn4","Output":"out"}],"Env":{"PATH":"{{ 2oscpeuv4wsscndhcbxero2sabmdbzn4:out }}","busybox_download":"{{ 2oscpeuv4wsscndhcbxero2sabmdbzn4:out }}"},"Name":"busybox","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"ty3hywqnh56vbn26543ca2mo7ucthqug":{"Args":["-c","\n            set -ex\n            cp -r $src/usr/* $out\n            mkdir test\n            cd test\n            $out/bin/git --version\n            "],"Builder":"{{ qqfse23hmwjw4v3rw2yoa6l7cdgcobda:out }}/bin/sh","Dependencies":[{"Hash":"qqfse23hmwjw4v3rw2yoa6l7cdgcobda","Output":"out"},{"Hash":"wcunj6ugkakaqrejz77jbfoojqxgwzou","Output":"out"}],"Env":{"PATH":"{{ qqfse23hmwjw4v3rw2yoa6l7cdgcobda:out }}/bin","src":"{{ wcunj6ugkakaqrejz77jbfoojqxgwzou:out }}"},"Name":"git","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"wcunj6ugkakaqrejz77jbfoojqxgwzou":{"Args":null,"Builder":"fetch_url","Dependencies":null,"Env":{"url":"http://s.minos.io/archive/bifrost/x86_64/git-2.10.2-1.tar.gz"},"Name":"git-2.10.2-1.tar.gz","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}}}}`

	var output project.ExecModuleOutput

	if err = json.Unmarshal([]byte(execOutputJSON), &output); err != nil {
		return
	}

	drvs, err := b.runBuildFromOutput(output)
	if err != nil {
		return
	}
	drv := drvs[0]

	return b.store.RunDerivation(context.Background(), drv, opts)
}
