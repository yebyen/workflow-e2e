package _tests_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"path"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

const (
	deisWorkflowServiceHost = "DEIS_WORKFLOW_SERVICE_HOST"
	deisWorkflowServicePort = "DEIS_WORKFLOW_SERVICE_PORT"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func getRandAppName() string {
	return fmt.Sprintf("test-%d", rand.Intn(1000))
}

func TestTests(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tests Suite")
}

var (
	testAdminUser     = fmt.Sprintf("test-admin-%d", rand.Intn(1000))
	testAdminPassword = "asdf1234"
	testAdminEmail    = fmt.Sprintf("test-admin-%d@deis.io", rand.Intn(1000))
	testUser          = fmt.Sprintf("test-%d", rand.Intn(1000))
	testPassword      = "asdf1234"
	testEmail         = fmt.Sprintf("test-%d@deis.io", rand.Intn(1000))
	url               = getController()
)

var _ = BeforeSuite(func() {
	// use the "deis" executable in the search $PATH
	_, err := exec.LookPath("deis")
	Expect(err).NotTo(HaveOccurred())

	// register the test-admin user
	register(url, testAdminUser, testAdminPassword, testAdminEmail)
	// verify this user is an admin by running a privileged command
	sess, err := start("deis users:list")
	Expect(err).To(BeNil())
	Eventually(sess).Should(gexec.Exit(0))

	// register the test user and add a key
	register(url, testUser, testPassword, testEmail)
	createKey("deis-test")
	sess, err = start("deis keys:add ~/.ssh/deis-test.pub")
	Expect(err).To(BeNil())
	Eventually(sess).Should(gexec.Exit(0))
	Eventually(sess).Should(gbytes.Say("Uploading deis-test.pub to deis... done"))
})

var _ = AfterSuite(func() {
	// cancel the test user
	cancel(url, testUser, testPassword)

	// cancel the test-admin user
	cancel(url, testAdminUser, testAdminPassword)
})

func register(url, username, password, email string) {
	sess, err := start("deis register %s --username=%s --password=%s --email=%s", url, username, password, email)
	Expect(err).To(BeNil())
	Eventually(sess).Should(gbytes.Say("Registered %s", username))
	Eventually(sess).Should(gbytes.Say("Logged in as %s", username))
}

func cancel(url, username, password string) {
	// log in to the account
	login(url, username, password)

	// cancel the account
	sess, err := start("deis auth:cancel --username=%s --password=%s --yes", username, password)
	Expect(err).To(BeNil())
	Eventually(sess).Should(gexec.Exit(0))
	Eventually(sess).Should(gbytes.Say("Account cancelled"))
}

func login(url, user, password string) {
	sess, err := start("deis login %s --username=%s --password=%s", url, user, password)
	Expect(err).To(BeNil())
	Eventually(sess).Should(gexec.Exit(0))
	Eventually(sess).Should(gbytes.Say("Logged in as %s", user))
}

func logout() {
	sess, err := start("deis auth:logout")
	Expect(err).To(BeNil())
	Eventually(sess).Should(gexec.Exit(0))
	Eventually(sess).Should(gbytes.Say("Logged out\n"))
}

// execute executes the command generated by fmt.Sprintf(cmdLine, args...) and returns its output as a cmdOut structure.
// this structure can then be matched upon using the SucceedWithOutput matcher below
func execute(cmdLine string, args ...interface{}) (string, error) {
	var stdout, stderr bytes.Buffer
	var cmd *exec.Cmd
	cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf(cmdLine, args...))
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stderr.String(), err
	}
	return stdout.String(), nil
}

func start(cmdLine string, args ...interface{}) (*gexec.Session, error) {
	cmdStr := fmt.Sprintf(cmdLine, args...)
	fmt.Println(cmdStr)
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	return gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
}

func createKey(name string) {
	var home string
	if user, err := user.Current(); err != nil {
		home = "~"
	} else {
		home = user.HomeDir
	}
	path := path.Join(home, ".ssh", name)
	// create the key under ~/.ssh/<name> if it doesn't already exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		sess, err := start("ssh-keygen -q -t rsa -b 4096 -C %s -f %s -N ''", name, path)
		Expect(err).To(BeNil())
		Eventually(sess).Should(gexec.Exit(0))
	}
	// add the key to ssh-agent
	sess, err := start("eval $(ssh-agent) && ssh-add %s", path)
	Expect(err).To(BeNil())
	Eventually(sess).Should(gexec.Exit(0))
}

func getController() string {
	host := os.Getenv(deisWorkflowServiceHost)
	if host == "" {
		panicStr := fmt.Sprintf(`Set %s to the workflow controller hostname for tests, such as:

$ %s=deis.10.245.1.3.xip.io make test-integration`, deisWorkflowServiceHost, deisWorkflowServiceHost)
		panic(panicStr)
	}
	port := os.Getenv(deisWorkflowServicePort)
	switch port {
	case "443":
		return "https://" + host
	case "80", "":
		return "http://" + host
	default:
		return fmt.Sprintf("http://%s:%s", host, port)
	}
}
