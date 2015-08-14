package main

import (
	"sort"
	"strings"
	"fmt"
	"net/http"
	"encoding/json"
	"io/ioutil"
	"crypto/tls"
	"log"
        "os"
        "os/exec"
)

/*
 * This go file is made to use the http://apt.dockerproject.org/ to get different available versions of
 * docker-engine. It also needs access to https://raw.githubusercontent.com/lenoyavidan/dockerfiles/master/dind-with-ssh-jenkins/Dockerfile 
 * to get a Dockerfile used to build with and it needs access to a dockerhub namespace and repo to get tags from and push images to.
 * The files jenkins-slave-startup.sh and wrapdocker are also needed for this to run
 *
 * When run, tags from the specified dockerhub namespace/repo will be taken to check to see which versions
 * of docker-engine have been built. These will be used to check against the available versions from apt.dockerproject.org
 * to see if any of the available versions have not yet been built and pushed to the repo. If there is at least
 * one version that hasn't been built, the Dockerfile will be changed to build that version and it will be built/pushed
 * to the namespace/repo. If there are multiple versions to be built, it will only build on at a time. In other words
 * every time you run this it will build only ONE version, so you must run it multiple times to build multiple versions.
 * 
 * NOTE: After running this, wait at least TEN minutes before running again. It takes time for the api call for getting 
 * tags to update properly, so if you run this again within ten minutes it will build and push the same version as last run
 *
 * There are 5 environmental variables that must be set for this to run correctly
 * 1. VERSION_TYPE: main, testing or experimental
 * 2. NAMESPACE: the proper dockerhub namespace
 * 3. REPO: the proper dockerhub repo
 * 4. PASSWORD: the password for your dockerhub namespace
 * 5. EMAIL: the email account for your dockerhub namespace
 * There is also an optional 6th variable that must be unset if it isn't going to be used
 * 6. VERSION_NUMBER: the specific version of the specified VERSION_TYPE that will be built if available
 */

var (
        name = os.Getenv("VERSION_TYPE") // type of version to get: main, testing or experimental
        namespace = os.Getenv("NAMESPACE") // name of hub namespace
        repo = os.Getenv("REPO") // name of hub repo
        password = os.Getenv("PASSWORD") // password for hub repo
        email = os.Getenv("EMAIL") // email for hub repo
        vers = os.Getenv("VERSION_NUMBER") // specific version number
	
	tr = &http.Transport {
		TLSClientConfig: &tls.Config {
		InsecureSkipVerify: true,
		},
	}
	client = &http.Client{Transport: tr} // creating a client to make api calls
)

// checks to make sure all environmental variables are set
func init() {
	if name == "" {
		log.Fatal("$VERSION_TYPE not set")
	}
	if namespace == "" {
		log.Fatal("$NAMESPACE not set")
	}
	if repo == "" {
		log.Fatal("$REPO not set")
	}
	if password == "" {
		log.Fatal("$PASSWORD not set")
	}
	if email == "" {
		log.Fatal("$EMAIL not set")
	}
}

// type for the tags to be retrieved from docker hub
type Tag struct {
        Layer string `json:"layer"`
	Name  string `json:"name"` // tag name
}

// changes value returned when printing tag value
func (tag Tag) String() string {
	return fmt.Sprintf("layer: %s, name: %s", tag.Layer, tag.Name)
}

/*
 * Searches through the given array of strings to see if the given string
 * is in the array
 * Parameter 1: an array of type string to be searched
 * Parameter 2: a value to search the array for
 * Return: If the value is found in the array, the index is returned. If it is not
 * found then -1 is returned
 * 
 * The function could be made more efficient by requiring the array
 * be sorted using sort.Strings before being passed in and then
 * performing a binary search rather than a linear search.
 * However, this would only be necessary for arrays that contained a hundred
 * strings or more since that is when the efficiency of the two methods begins to 
 * greatly diverge
 */
func FindString(str []string, val string) int {
	for i, v := range str {
		if v == val {
			return i
		} 
	}
	return -1
}

/*
 * Takes in a string and searches through it to find the version number
 * Parameter: a string to be searched for the keyword "Version:"
 * Return: It will return a string of the version number, or if it doesn't
 * find the version number, the function returns an empty string ""
 *
 * This function is used to parse through the Packages file on the apt.dockerproject.org  
 * site for the most recent version of docker-engine
 */
func Version(str string) string {
	strarr := strings.Fields(str)
	for i, v := range strarr {
		if strings.EqualFold(v, "Version:") { 
			version := strarr[i + 1] // if the current string is "Version:" get the next string which should be the version number
			return strings.Replace((strings.Split(version, "-0"))[0], "~", "-", 1) // get rid of unnecessary text and replace the tilda with a dash
		}
	}
        return ""
}

/*
 * This Function uses api calls and the Version function to get the latest version of docker-engine
 * Return 1: A string of the latest version of docker-engine
 * Return 2: An error that is nil if no error occured
 */
func NewestVersion() (str string, err error) {
	resp, err := client.Get(fmt.Sprintf("https://apt.dockerproject.org/repo/dists/ubuntu-trusty/%s/binary-amd64/Packages", name))
	if err != nil { return }
	defer resp.Body.Close()

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil { return }
	str = Version(string(contents)) // call function to parse variable contents for the version
	return
}

/*
 * Function uses api call to get tag names from given docker hub namespace and repo
 * Return 1: A slice of strings representing the tag names from the specifieddocker hub repo
 * Return 2: An error code that is nil if no error occured
 */
func Tags() (list []string, err error) {
        var tag []Tag
        list = make([]string, 0)
        resp2, err := client.Get(fmt.Sprintf("https://registry.hub.docker.com/v1/repositories/%s/%s/tags", namespace, repo))
	if err != nil { return }
	defer resp2.Body.Close()
	if err = json.NewDecoder(resp2.Body).Decode(&tag); err != nil { return }

        for i := range tag { // go through array of tags to get the tag names and add them to an array
                list = append(list, tag[i].Name) 
	}
        sort.Strings(list) // sort the list of tag names
	return
} 

/*
 * Function that gets all the current versions available from apt.dockerproject.org 
 * site for the given build
 * Parameter: a string indicating which build to choose: main, testing or experimental
 * Return 1: A slice of strings containing all the versions of docker-engine available to build
 * Return 2: An error that is nil if no errors occured 
 * 
 * Currently there is no html parser, would be good to add if a stable version is created
 */
func AvailableVersions(build string) (versions []string, err error) {
	versions = make([]string, 0)
        resp3, err := client.Get(fmt.Sprintf("http://apt.dockerproject.org/repo/pool/%s/d/docker-engine/", build))
	if err != nil { return }
        defer resp3.Body.Close() 
	text, err := ioutil.ReadAll(resp3.Body)
	if err != nil { return }
	nums := strings.Fields(string(text))

	for _, v := range nums {
		if strings.Contains(v, "docker-engine_") { // only get strings that have the version number in it
			value := (strings.Split((strings.Split(v, "_"))[1], "-"))[0] // split the the string out so that only the version is returned 
			value = strings.Replace(value, "~", "-", 1)
			if FindString(versions, value) < 0 { 
				versions = append(versions, value) // if the version hasn't already been added, add it to the array
			}
		}
	}

	if name == "experimental" {
		temp := make([]string, 0)
		sort.Sort(sort.Reverse(sort.StringSlice(versions)))
		temp = append(temp, versions[0])
		versions = temp
	}
	return
}

/*
 * Function that downloads the necessary Dockerfile and then changes it to build the image with the 
 * correct version of docker-engine. It then builds the image and returns the image name and tag in one string
 * Parameter: A string representing the version number to build
 * Return 1: A string that contains the name of the built image and its corresponding tag ex: "bmangold/dind-with-ssh:1.8.0"
 * Return 2: An error that is nil if no error occurred 
 */
func BuildVersion(version string) (image string, err error) {
	output, err := exec.Command("curl", "-sS", "https://raw.githubusercontent.com/lenoyavidan/dockerfiles/master/dind-with-ssh-jenkins/Dockerfile").CombinedOutput()
	if err != nil { return }
	dockerfile, err := os.Create("Dockerfile")
	_, err = dockerfile.Write(output)
	if err != nil { return }
	
	output, err = exec.Command("curl", "-sS", "https://raw.githubusercontent.com/jpetazzo/dind/master/wrapdocker").CombinedOutput()
	if err != nil { return }
	wrapdocker, err := os.Create("wrapdocker")
	_, err = wrapdocker.Write(output)
	if err != nil { return }

	err = exec.Command("sed", "-i", fmt.Sprintf("/RUN curl/ i\\ENV TYPE %s", name), "Dockerfile").Run()
	if err != nil { return }
	err = exec.Command("sed", "-i", fmt.Sprintf("/RUN curl/ i\\ENV DEB_FILE docker-engine_%s-0~trusty_amd64.deb", strings.Replace(version, "-", "~", 1)), "Dockerfile").Run()
	if err != nil { return }
	err = exec.Command("sed", "-i", "/RUN curl/ c\\RUN curl -sS http://apt.dockerproject.org/repo/pool/$TYPE/d/docker-engine/$DEB_FILE > deb/$DEB_FILE", "Dockerfile").Run()
	if err != nil { return }
	err = exec.Command("sed", "-i", "/RUN curl/ i\\RUN mkdir deb", "Dockerfile").Run()
	if err != nil { return }
	err = exec.Command("sed", "-i", "/RUN curl/ a\\RUN dpkg -i deb/$DEB_FILE", "Dockerfile").Run()
	if err != nil { return }

	// add something to change tag name for experimental versions since its name needs to be split so the proper tag name can be retrieved
	if name == "experimental" {
		image = fmt.Sprintf("%s/%s:%s", namespace, repo, strings.Split(version, "~")[0])
	} else if name == "testing" && !strings.Contains(version, "rc") {
		image = fmt.Sprintf("%s/%s:%s", namespace, repo, version + "-rc1")
	} else {
		image = fmt.Sprintf("%s/%s:%s", namespace, repo, version)
	}
	err = exec.Command("sudo", "docker", "build", "-t", image, ".").Run()
	if err != nil { return }

	return
}

/*
 * This function uses the docker run command to check that the image can run as a container and
 * that the version of docker it is running on matches the passed in version
 * Parameter: the string representing the name and tag of the image to be run and checked
 * Return 1: A boolean value that is true if the image built correctly and the versions match,
 * otherwise it is false
 * Return 2: An error that is nil if no errors occured
 */
func BuildWorks(image string) (works bool, err error) {
	works = false
	output, err := exec.Command("sudo", "docker", "run", "--rm", "--privileged", "-e", "LOG=file", image, "bash", "-c", "(/usr/local/bin/wrapdocker &);sleep 5;docker version").CombinedOutput()
	if err != nil { return }

	arr := strings.Fields(string(output)) // divide up the output into an array of strings to get the individual string values to check with
	for i, v := range arr {
		if v == "Version:" || v == "version:" && (arr[i - 1] == "Server" || arr[i - 1] == "Client") { // the first test is for docker versions 1.8.0 and later, the rest of the tests are a work around to test the 1.7.0 and 1.7.1 versions
			version := arr[i + 1]
			if version == strings.Split(image, ":")[1] { // checking that the version numbers match
				works = true
			} else {
				return false, nil
			}
		}
	} 

	return
}

func main() {

    	latest, err := NewestVersion() // get the latest available version number of docker-engine from the package on the apt site
	if err != nil {
		fmt.Println("failed to get latest available version")
		log.Fatal(err)
	}
	if latest != "" {
            fmt.Printf("latest docker-engine version: %s\n", latest)
	}

	list, err := Tags() // get the tags from the given namespace/repo to see which versions have already been built
	if err != nil {
		fmt.Println("failed to retrieve tags")
		log.Fatal(err)
	}
        fmt.Printf("tags pulled from %s/%s: ", namespace, repo)
        for _, v := range list {
        	fmt.Printf("%s ", v)
	}
        fmt.Println()

	versions, err := AvailableVersions(name) // get all versions from apt site that can be downloaded and built
	if err != nil {
		fmt.Println("failed to get available versions")
		log.Fatal(err)
	}
	fmt.Printf("available docker-engine versions are: %v\n", versions)

	// if a specific version is specified to be built, set it up so it is the version to be built
	if FindString(versions, vers) >= 0 {
		versions[0] = vers
	} else if vers != "" {
		fmt.Printf("specified version %s not available to build\n", vers)
		return
	}

	for _, v := range versions {
		changed := "false"
		if name == "testing" && !strings.Contains(v, "rc") {
			changed = v
			v += "-rc1"
		}
		// test to see if the version has already been built
		// the second test in the if statement is for experimental since its string has to be handled differently
		if FindString(list, v) < 0 && FindString(list, strings.Split(v, "~")[0]) < 0 {
			fmt.Printf("logging in to %s\n", namespace)
			err = exec.Command("sudo", "docker", "login", fmt.Sprintf("-u=%s", namespace), fmt.Sprintf("-p=%s", password), fmt.Sprintf("-e=%s", email)).Run()
			if err != nil {
				fmt.Println("login failed")
				log.Fatal(err)
			}
			
			fmt.Printf("building docker-engine version %s\n", v)
			if changed != "false" {
				v = changed
			}
			image, err := BuildVersion(v) // build version
			if err != nil {
				fmt.Println("build failed")
				log.Fatal(err)
			}

			img := image
			if changed != "false" { 
				img = fmt.Sprintf("%s/%s:%s", namespace, repo, v) // needed for when the version is in testing and doesn't have -rc#
			}
			works, err := BuildWorks(img) // returns true if image works and has correct docker version
			if err != nil || !works {
				fmt.Printf("image %s not built properly\n", image)
				log.Fatal(err)
			}
			fmt.Println("build succeeded")

			if v == latest {
				var retag string
				if name == "main" {
					retag = fmt.Sprintf("%s/%s:latest", namespace, repo)	
					fmt.Printf("pushing latest to %s/%s\n", namespace, repo)
				} else if name == "testing" {
					retag = fmt.Sprintf("%s/%s:rc-latest", namespace, repo)	
					fmt.Printf("pushing rc-latest to %s/%s\n", namespace, repo)
				} else if name == "experimental" {
					retag = fmt.Sprintf("%s/%s:dev-latest", namespace, repo)	
					fmt.Printf("pushing dev-latest to %s/%s\n", namespace, repo)
				}
				err = exec.Command("sudo", "docker", "tag", "-f", image, retag).Run() // tag the image as a type of latest
				if err != nil {
					fmt.Println("latest tag failed")
					log.Fatal(err)
				}
				err = exec.Command("sudo", "docker", "push", retag).Run() // tag the image as latest and push it as latest
				if err != nil {
					fmt.Println("latest push failed")
					log.Fatal(err)
				}
			}
			if name == "experimental" {
				break
			} 
			fmt.Printf("pushing docker-engine version %s to %s/%s\n", v, namespace, repo)
			err = exec.Command("sudo", "docker", "push", image).Run() // push the image
			if err != nil {
				fmt.Println("push failed")
				log.Fatal(err)
			}
			break
		} else {
			fmt.Printf("version %s already built and pushed\n", v)
			if vers != "" {
				break
			}
		}
	} 
}
