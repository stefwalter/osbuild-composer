package blueprint

import "github.com/osbuild/osbuild-composer/internal/pipeline"

type qcow2Output struct{}

func (t *qcow2Output) translate(b *Blueprint) *pipeline.Pipeline {
	p := &pipeline.Pipeline{
		BuildPipeline: getF30BuildPipeline(),
	}

	options := &pipeline.DNFStageOptions{
		ReleaseVersion:   "30",
		BaseArchitecture: "x86_64",
	}
	options.AddRepository(getF30Repository())
	packages := [...]string{"kernel-core",
		"@Fedora Cloud Server",
		"chrony",
		"polkit",
		"systemd-udev",
		"selinux-policy-targeted",
		"grub2-pc",
		"langpacks-en"}
	for _, pkg := range packages {
		options.AddPackage(pkg)
	}
	excludedPackages := [...]string{"dracut-config-rescue",
		"etables",
		"firewalld",
		"gobject-introspection",
		"plymouth"}
	for _, pkg := range excludedPackages {
		options.ExcludePackage(pkg)
	}
	p.AddStage(pipeline.NewDNFStage(options))
	addF30LocaleStage(p)
	addF30FSTabStage(p)
	addF30GRUB2Stage(p)
	addF30SELinuxStage(p)
	addF30FixBlsStage(p)
	addF30QemuAssembler(p, "qcow2", t.getName())

	return p
}

func (t *qcow2Output) getName() string {
	return "image.qcow2"
}

func (t *qcow2Output) getMime() string {
	return "application/x-qemu-disk"
}
