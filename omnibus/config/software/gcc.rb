#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# expeditor/ignore: deprecated 2022-03

# do_not_auto_update
name "gcc"
default_version "8.4.0"

dependency "gmp"
dependency "mpfr"
dependency "mpc"
dependency "libiconv"

# version_list: url=https://ftp.gnu.org/gnu/gcc/ filter=*.tar.gz
version("4.9.2") { source sha256: "3e573826ec8b0d62d47821408fbc58721cd020df3e594cd492508de487a43b5e" }
version("4.9.3") { source sha256: "e6c63b40877bc756cc7cfe6ca98013eb15f02ec6c8c2cf68e24533ad1203aaba" }
version("4.9.4") { source sha256: "1680f92781b92cbdb57d7e4f647c650678c594154cb0d707fd9a994424a9860d" }
version("5.3.0") { source sha256: "b7f5f56bd7db6f4fcaa95511dbf69fc596115b976b5352c06531c2fc95ece2f4" }
version("5.5.0") { source sha256: "3aabce75d6dd206876eced17504b28d47a724c2e430dbd2de176beb948708983" }
version("6.5.0") { source sha256: "4eed92b3c24af2e774de94e96993aadbf6761cdf7a0345e59eb826d20a9ebf73" }
version("7.5.0") { source sha256: "4f518f18cfb694ad7975064e99e200fe98af13603b47e67e801ba9580e50a07f" }
version("8.4.0") { source sha256: "41e8b145832fc0b2b34c798ed25fb54a881b0cee4cd581b77c7dc92722c116a8" }
version("9.3.0") { source sha256: "5258a9b6afe9463c2e56b9e8355b1a4bee125ca828b8078f910303bc2ef91fa6" }
version("10.2.0") { source sha256: "27e879dccc639cd7b0cc08ed575c1669492579529b53c9ff27b0b96265fa867d" }

source url: "https://mirrors.kernel.org/gnu/gcc/gcc-#{version}/gcc-#{version}.tar.gz"

relative_path "gcc-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_command = ["./configure",
                     "--prefix=#{install_dir}/embedded",
                     "--disable-nls",
                     "--enable-languages=c,c++",
                     "--disable-multilib"]

  command configure_command.join(" "), env: env
  # gcc takes quite a long time to build (over 2 hours) so we're setting the mixlib shellout
  # timeout to 4 hours. It's not great but it's required (on solaris at least, need to verify
  # on any other platforms we may use this with)
  # gcc also has issues on a lot of platforms when running a multithreaded job,
  # so unfortunately we have to build with 1 worker :(
  make "-j #{workers}", env: env, timeout: 14400
  make "install", env: env
end
