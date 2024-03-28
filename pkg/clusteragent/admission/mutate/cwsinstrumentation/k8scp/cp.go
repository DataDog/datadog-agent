package k8scp

import (
	"archive/tar"
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	"os"
)

// Copy perform remote copy operations
type Copy struct {
	Container string
	Namespace string

	config    *restclient.Config
	clientSet kubernetes.Interface

	streams genericiooptions.IOStreams
	in      *bytes.Buffer
	out     *bytes.Buffer
	errOut  *bytes.Buffer
}

// NewCopy creates a Command instance
func NewCopy(config *restclient.Config, clientSet kubernetes.Interface) *Copy {
	config.APIPath = "/api"
	config.GroupVersion = &schema.GroupVersion{Version: "v1"}
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs}
	ioStreams, in, out, errOut := genericiooptions.NewTestIOStreams()
	return &Copy{
		streams:   ioStreams,
		in:        in,
		out:       out,
		errOut:    errOut,
		config:    config,
		clientSet: clientSet,
	}
}

func (o *Copy) CopyToPod(localFile string, remoteFile string, pod *corev1.Pod, container string) error {
	// sanity check
	if _, err := os.Stat(localFile); err != nil {
		return fmt.Errorf("%s doesn't exist in local filesystem", localFile)
	}

	reader, writer := io.Pipe()
	srcFile := newLocalPath(localFile)
	destFile := newRemotePath(remoteFile)

	go func(src localPath, dest remotePath, writer io.WriteCloser) {
		defer writer.Close()
		if err := makeTar(src, dest, writer); err != nil {
			log.Debugf("failed to tar local file: %v", err)
		}
	}(srcFile, destFile, writer)

	// arguments are split on purpose to differentiate this kubectl cp from others
	cmdArr := []string{"tar", "-x", "-m", "-f", "-"}
	destFileDir := destFile.Dir().String()
	if len(destFileDir) > 0 {
		cmdArr = append(cmdArr, "-C", destFileDir)
	}

	options := &ExecOptions{}
	options.StreamOptions = StreamOptions{
		IOStreams: genericiooptions.IOStreams{
			In:     reader,
			Out:    o.out,
			ErrOut: o.errOut,
		},
		Stdin:         true,
		Namespace:     pod.GetNamespace(),
		PodName:       pod.GetName(),
		ContainerName: container,
	}
	options.Command = cmdArr
	options.Config = o.config
	options.Executor = &DefaultRemoteExecutor{}

	return options.Run(pod)
}

func makeTar(src localPath, dest remotePath, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	srcPath := src.Clean()
	destPath := dest.Clean()
	return recursiveTar(srcPath.Dir(), srcPath.Base(), destPath.Dir(), destPath.Base(), tarWriter)
}

func recursiveTar(srcDir, srcFile localPath, destDir, destFile remotePath, tw *tar.Writer) error {
	matchedPaths, err := srcDir.Join(srcFile).Glob()
	if err != nil {
		return err
	}
	for _, fpath := range matchedPaths {
		stat, err := os.Lstat(fpath)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			files, err := os.ReadDir(fpath)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				//case empty directory
				hdr, _ := tar.FileInfoHeader(stat, fpath)
				hdr.Name = destFile.String()
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
			}
			for _, f := range files {
				if err := recursiveTar(srcDir, srcFile.Join(newLocalPath(f.Name())),
					destDir, destFile.Join(newRemotePath(f.Name())), tw); err != nil {
					return err
				}
			}
			return nil
		} else if stat.Mode()&os.ModeSymlink != 0 {
			//case soft link
			hdr, _ := tar.FileInfoHeader(stat, fpath)
			target, err := os.Readlink(fpath)
			if err != nil {
				return err
			}

			hdr.Linkname = target
			hdr.Name = destFile.String()
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		} else {
			//case regular file or other file type like pipe
			hdr, err := tar.FileInfoHeader(stat, fpath)
			if err != nil {
				return err
			}
			hdr.Name = destFile.String()

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			return f.Close()
		}
	}
	return nil
}
